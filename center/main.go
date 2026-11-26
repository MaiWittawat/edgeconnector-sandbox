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
	"encoding/json"
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
	"github.com/MaiWittawat/edgeconnector/connectorserver/api"
	"github.com/MaiWittawat/edgeconnector/connectorserver/fleet"
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

	// Event ที่ plugin demo ส่งขึ้นมาเมื่อทำ Command job.run เสร็จ · type ที่ไม่มี handler ก็ถูกเก็บอยู่แล้ว
	// handler นี้แค่ log ให้เห็น (center จริงบันทึกผลงาน และกันซ้ำด้วย ConnectorID+Seq หรือ job_id)
	srv.RegisterEvent("job.done", func(ctx context.Context, ev connectorserver.Event) error {
		log.Printf("event job.done from %s seq %d: %s", ev.ConnectorID, ev.Seq, ev.Payload)
		return nil
	})

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
	// AdminHandler ไม่มีทางส่ง Notify จึงเพิ่มที่นี่ไว้ลอง (pattern ที่เจาะจงกว่าชนะ /admin/)
	mux.HandleFunc("POST /admin/connectors/{id}/notify", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || body.Type == "" {
			http.Error(w, `body must be {"type": "...", "payload": ...}`, http.StatusBadRequest)
			return
		}
		// payload ที่เป็น JSON string ส่งเป็นข้อความล้วน (ไม่มีเครื่องหมายคำพูด) · อย่างอื่นส่งตามที่ได้มา
		payload := []byte(body.Payload)
		var text string
		if json.Unmarshal(body.Payload, &text) == nil {
			payload = []byte(text)
		}
		if err := srv.Notify(r.Context(), r.PathValue("id"), body.Type, payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
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

// newUI = ฝังหน้าจัดการที่ /ui/ และ JSON API ที่ /ui/api/v1/: login ด้วย Basic auth รหัสเดียว เห็น tenant เดียว
// (center จริงใช้ session ของตัวเองใน Authorize แล้วบอก tenant/สิทธิ์ของ user คนนั้น)
// ทั้งสองใช้ fleet.Config ก้อนเดียว: login, registry และกฎสิทธิ์จึงเหมือนกันเสมอ
func newUI(srv *connectorserver.Server, password, tenant, registry string) (http.Handler, error) {
	login := func(r *http.Request) (string, bool) {
		user, pass, ok := r.BasicAuth()
		return user, ok && user != "" && subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1
	}
	shared := fleet.Config{
		ReleaseURL: registry,
		Authorize: func(r *http.Request) (fleet.User, error) {
			user, ok := login(r)
			if !ok {
				return fleet.User{}, errors.New("wrong password")
			}
			return fleet.User{TenantID: tenant, UserID: user, Name: user, CanManage: true}, nil
		},
	}
	pages, err := ui.New(srv, ui.Config{Config: shared, BasePath: "/ui"})
	if err != nil {
		return nil, err
	}
	apiHandler, err := api.New(srv, api.Config{Config: shared, BasePath: "/ui"})
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	// ให้ browser ถามรหัสเฉพาะหน้าจัดการ · API ตอบ 401 เป็น JSON ให้โปรแกรมอ่านเอง
	mux.Handle("/ui/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := login(r); !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="sandbox", charset="UTF-8"`)
		}
		pages.ServeHTTP(w, r)
	}))
	mux.Handle("/ui/api/", apiHandler) // pattern ยาวกว่าชนะ: /ui/api/… ไม่ผ่านหน้าจัดการ
	return mux, nil
}
