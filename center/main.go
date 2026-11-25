// center = center ตัวอย่างสำหรับ sandbox: เขียนเหมือนที่ผู้ใช้ภายนอกจะฝัง connectorserver ในระบบของตัวเอง
// ใช้เฉพาะ API สาธารณะของ edgeconnector (connectorserver, store/open, ui) ไม่แตะ internal/
//
//	go run ./center -ui-password secret
//
// แล้วเปิด http://127.0.0.1:8080/ui/ (user อะไรก็ได้ รหัสตามที่ตั้ง)
//
// ใช้ TLS ปิด (Insecure) เพราะเป็นการทดสอบในเครื่อง · center จริงต้องเปิด TLS (ดู docs ของ connectorserver.Config)
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/MaiWittawat/edgeconnector/connectorserver"
	"github.com/MaiWittawat/edgeconnector/connectorserver/store"
	"github.com/MaiWittawat/edgeconnector/connectorserver/store/open"
	"github.com/MaiWittawat/edgeconnector/connectorserver/ui"
)

// registry ที่ manager ใน sandbox ใช้ (ที่เดียวกับ CC_RELEASE_URL ของ manager) UI อ่าน index.json จากที่นี่มาแสดงเป็นรายการ plugin
const defaultRegistry = "https://raw.githubusercontent.com/MaiWittawat/edgeconnector-registry/main"

func main() {
	grpcAddr := flag.String("addr", ":7443", "ที่อยู่ gRPC ที่ manager ต่อเข้ามา (container ต่อผ่าน host.docker.internal จึงต้องไม่ใช่ 127.0.0.1)")
	adminAddr := flag.String("admin", "127.0.0.1:8080", "ที่อยู่ UI + admin HTTP (admin ไม่มี login ห้ามเปิดให้เครือข่ายเห็น)")
	dataDir := flag.String("data", "data/center", "โฟลเดอร์เก็บข้อมูล (store.json)")
	password := flag.String("ui-password", "", "รหัสผ่านของหน้า UI (จำเป็น)")
	tenant := flag.String("tenant", "t1", "tenant ที่หน้า UI แสดง")
	registry := flag.String("release-url", defaultRegistry, "registry ของ plugin")
	flag.Parse()
	if *password == "" {
		log.Fatal("ต้องตั้ง -ui-password")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatal(err)
	}
	st, err := open.Open(ctx, open.Config{Driver: store.MemDriver, DSN: filepath.Join(*dataDir, "store.json")})
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	srv := connectorserver.New(connectorserver.Config{Addr: *grpcAddr, Store: st, Insecure: true})

	// Call ที่ plugin เรียกขึ้นมาหา center · ต้องมี scope dnc:read จึงเรียกได้ ชื่อ scope นี้จะโผล่ในหน้า UI ให้เลือก
	srv.Handle("dnc.check", func(ctx context.Context, call connectorserver.Call) ([]byte, error) {
		return []byte("clean"), nil
	}, connectorserver.RequireScope("dnc:read"))

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
	log.Printf("gRPC listening on %s", srv.Addr())

	uiHandler, err := newUI(srv, *password, *tenant, *registry)
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/admin/", srv.AdminHandler())
	mux.Handle("/ui/", uiHandler)
	web := &http.Server{Addr: *adminAddr, Handler: mux}
	go func() {
		log.Printf("UI on http://%s/ui/", *adminAddr)
		if err := web.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = web.Shutdown(shutdownCtx)
	srv.Stop(shutdownCtx)
}

// newUI = ฝังหน้าจัดการ: login ด้วย Basic auth รหัสเดียว เห็น tenant เดียว
// (center จริงใช้ session ของตัวเองใน Authorize แล้วบอก tenant/สิทธิ์ของ user คนนั้น)
func newUI(srv *connectorserver.Server, password, tenant, registry string) (http.Handler, error) {
	login := func(r *http.Request) (string, bool) {
		user, pass, ok := r.BasicAuth()
		return user, ok && user != "" && subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1
	}
	h, err := ui.New(srv, ui.Config{
		BasePath:   "/ui",
		ReleaseURL: registry,
		Authorize: func(r *http.Request) (ui.User, error) {
			user, ok := login(r)
			if !ok {
				return ui.User{}, errors.New("wrong password")
			}
			return ui.User{TenantID: tenant, UserID: user, Name: user, CanManage: true}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := login(r); !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="sandbox", charset="UTF-8"`) // ให้ browser ถามรหัส
		}
		h.ServeHTTP(w, r)
	}), nil
}
