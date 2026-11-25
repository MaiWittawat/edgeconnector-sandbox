// verify = ตรวจ release ของ manager ก่อนใส่ลง image (ใช้ใน Dockerfile เท่านั้น ไม่ใช้ library ของ edgeconnector)
//
//	verify -manifest manifest.json -archive x.tar.gz -key lab-2026=<base64> -name manager -version 1.0.0 -os linux -arch amd64
//
// ตรวจสิ่งเดียวกับที่ manager ตรวจ plugin: sha256 ของไฟล์ตรงกับ manifest และลายเซ็น Ed25519 ถูกต้อง
// ข้อความที่เซ็นคือ "cc-artifact:<name>:<version>:<os>:<arch>:<sha256>" (รูปแบบเดียวกับ release/release.go ของ edgeconnector)
// ไม่ผ่าน = exit 1 → docker build ล้ม ไม่ได้ image ที่ไม่ได้ตรวจ
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type manifest struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Artifact struct {
		SHA256    string `json:"sha256"`
		Signature []byte `json:"signature"` // JSON base64
		KeyID     string `json:"keyId"`
	} `json:"artifact"`
}

func main() {
	manifestPath := flag.String("manifest", "", "manifest.json")
	archivePath := flag.String("archive", "", "ไฟล์ .tar.gz")
	key := flag.String("key", "", "key ที่เชื่อถือ id=base64")
	name := flag.String("name", "", "ชื่อที่คาดหวัง")
	version := flag.String("version", "", "version ที่คาดหวัง")
	goos := flag.String("os", "", "os ที่คาดหวัง")
	arch := flag.String("arch", "", "arch ที่คาดหวัง")
	flag.Parse()
	if err := run(*manifestPath, *archivePath, *key, *name, *version, *goos, *arch); err != nil {
		fmt.Fprintln(os.Stderr, "verify:", err)
		os.Exit(1)
	}
	fmt.Println("verify: ok")
}

func run(manifestPath, archivePath, key, name, version, goos, arch string) error {
	id, b64, ok := strings.Cut(key, "=")
	if !ok || id == "" {
		return fmt.Errorf("-key ต้องเป็น id=base64")
	}
	pub, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("key %q ไม่ใช่ Ed25519 public key", id)
	}

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	// manifest ต้องเป็นของ release ที่ขอ ไม่งั้น registry สลับไฟล์ข้าม version ได้
	if m.Name != name || m.Version != version || m.OS != goos || m.Arch != arch {
		return fmt.Errorf("manifest เป็นของ %s %s %s/%s ไม่ใช่ที่ขอ", m.Name, m.Version, m.OS, m.Arch)
	}
	if m.Artifact.KeyID != id {
		return fmt.Errorf("manifest เซ็นด้วย key %q ซึ่งไม่ได้ตั้งให้เชื่อถือ", m.Artifact.KeyID)
	}

	archive, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != m.Artifact.SHA256 {
		return fmt.Errorf("sha256 ของไฟล์ไม่ตรงกับ manifest")
	}
	msg := "cc-artifact:" + m.Name + ":" + m.Version + ":" + m.OS + ":" + m.Arch + ":" + m.Artifact.SHA256
	if !ed25519.Verify(pub, []byte(msg), m.Artifact.Signature) {
		return fmt.Errorf("ลายเซ็นไม่ถูกต้อง")
	}
	return nil
}
