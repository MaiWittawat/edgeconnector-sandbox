# edgeconnector-sandbox

ลอง Edge Connector แบบที่ผู้ใช้ภายนอกจะใช้: center ใช้แค่ API สาธารณะของ edgeconnector และเครื่องลูกค้าจำลองโหลด manager ที่เซ็นแล้วจาก registry เหมือนเครื่องจริง

| ส่วน | คืออะไร |
|---|---|
| `center/` | center ตัวอย่าง ใช้แค่ `connectorserver`, `store/open` และ `ui` · มี Call `dnc.check` ที่ต้องมี scope `dnc:read` และ log Event `job.done` (คู่กับ plugin `demo` ใน edgeconnector-plugin) · มีหน้าจัดการที่ `/ui/`, admin HTTP ที่ `/admin/` และ `POST /admin/connectors/{id}/notify` ไว้ส่ง Notify |
| `machine/` | เครื่องลูกค้าจำลอง (Docker image): โหลด manager จาก registry ตาม channel (ค่าเริ่มต้น `stable`) แล้วตรวจลายเซ็นก่อนใส่ image |
| `compose.yml` | รันเครื่องลูกค้าจำลองได้สามแบบ (ด้านล่าง) |
| `scripts/package-plugin.sh` | build plugin แล้วเซ็นลง registry ในเครื่องด้วย key ทิ้ง (แบบ 3) |

## เลือกแบบที่ใช้

| แบบ | เปิดอะไร | ใช้เมื่อ |
|---|---|---|
| 1. center ตัวอย่าง | `go run ./center` + container `machine` | ยังไม่มี center ของตัวเอง อยากเห็นทั้งวง |
| 2. center ของคุณเอง | container `machine` อย่างเดียว | ฝัง `connectorserver` ในระบบของคุณแล้ว อยากลอง plugin กับระบบจริง |
| 3. plugin ที่ยังไม่ปล่อย | แบบ 1 หรือ 2 + registry ในเครื่อง (container `registry` + `machine-local`) | กำลังเขียน plugin ยังไม่อยากปล่อยลง registry จริง |

plugin รันเดี่ยว ๆ ไม่ได้: plugin ได้ flag และตัวตนจาก manager (`worker.Run`) จึงต้องมีเครื่องลูกค้าจำลองที่มี manager เสมอ แล้วติดตั้ง plugin ผ่าน center

ต้องมี Docker และ Go · edgeconnector เป็น repo private: ตั้งให้ go อ่านได้ก่อน (ครั้งเดียว)

```bash
go env -w GOPRIVATE='github.com/MaiWittawat/*' GONOSUMDB='github.com/MaiWittawat/*'
cp .env.example .env
```

## แบบ 1: center ตัวอย่าง

```bash
# terminal 1: center · UI ที่ http://127.0.0.1:8080/ui/ (user อะไรก็ได้ รหัส secret)
go run ./center -ui-password secret
```

1. เปิด UI → Add manager → คัดลอก token ใส่ `TOKEN=` ใน `.env` (หรือ `curl -s -XPOST 127.0.0.1:8080/admin/enrollment-tokens -d '{"tenant_id":"t1","type":"manager"}'`)
2. `.env` ใช้ค่าตาม `.env.example` ได้เลย (`CENTER_ADDR=host.docker.internal:7443`, `INSECURE=true` เพราะ center ตัวอย่างไม่เปิด TLS)
3. เปิดเครื่องลูกค้าจำลอง

```bash
# terminal 2
docker compose up --build machine
```

4. ใน UI จะเห็น manager ขึ้นมา → Install plugin เช่น `hello` `1.0.0` จาก registry บน GitHub
5. หรือสั่งด้วย curl:

```bash
curl -s 127.0.0.1:8080/admin/connectors                                    # หา <manager-id>
curl -s -XPOST 127.0.0.1:8080/admin/connectors/<manager-id>/commands \
    -d '{"type":"plugin.install","payload":{"name":"hello","version":"1.0.0"}}'
curl -s 127.0.0.1:8080/admin/connectors                                    # มี connector ชนิด hello (plg_...) เพิ่ม
curl -s -XPOST 127.0.0.1:8080/admin/connectors/<plg-id>/requests -d '{"type":"version"}'   # → 1.0.0
```

admin HTTP ที่ `/admin/` ไม่มี login มีไว้ลองในเครื่องเท่านั้น ห้ามเปิดออกเครือข่าย

## แบบ 2: center ของคุณเอง

ไม่ต้องรัน `./center` · แก้ `.env`:

```bash
CENTER_ADDR=center.example.com:7443   # ที่อยู่ gRPC ของ connectorserver ใน center ของคุณ
INSECURE=false
CA=/certs/ca.pem                      # เฉพาะเมื่อ center ใช้ CA ส่วนตัว: วางไฟล์ไว้ที่ ./certs/ca.pem
TOKEN=<token ชนิด manager จาก center ของคุณ>
```

```bash
docker compose up --build machine
```

แล้วติดตั้ง plugin จากหน้าจัดการหรือ `RouteCommand(ctx, managerID, "plugin.install", ...)` ของ center คุณ · center ที่รันบนเครื่องเดียวกับ Docker ใช้ `CENTER_ADDR=host.docker.internal:<port>`

## แบบ 3: plugin ที่ยังไม่ปล่อย

build plugin แล้วเซ็นลง `./data/registry` ด้วย key ทิ้ง (`./data/keys/sandbox.key` สร้างครั้งแรกครั้งเดียว)

```bash
scripts/package-plugin.sh ../my-plugin my-plugin 0.1.0          # โฟลเดอร์ main ของ plugin
scripts/package-plugin.sh github.com/MaiWittawat/edgeconnector/examples/hello-plugin hello 0.1.0
```

script พิมพ์บรรทัด `LOCAL_TRUSTED_KEYS=sandbox=...` ให้ใส่ใน `.env` (ครั้งเดียว) แล้วเปิดเครื่องลูกค้าจำลองที่ใช้ registry ในเครื่อง:

```bash
docker compose --profile local up --build machine-local
```

ใช้กับ center ตัวอย่าง: รัน `go run ./center -ui-password secret -release-url http://127.0.0.1:8000` เพื่อให้หน้า UI แสดงรายการ plugin จาก registry ในเครื่อง · ใช้กับ center ของคุณ: ตั้ง `.env` แบบ 2 ได้เหมือนกัน

แก้ plugin แล้วลองใหม่ = `package-plugin.sh` ด้วย version ใหม่ แล้วสั่ง `plugin.install` version นั้น (registry อ่านไฟล์ใหม่ทันที ไม่ต้อง restart)

ทำไมต้องมี container `registry`: manager ยอมโหลดผ่าน `http://` เฉพาะ `127.0.0.1` / `localhost` เท่านั้น `machine-local` จึงใช้ network เดียวกับ `registry` แล้วโหลดที่ `http://127.0.0.1:8000`

## manager ใน container

| เรื่อง | รายละเอียด |
|---|---|
| version | ไม่ตั้ง `MANAGER_VERSION` = ตาม channel `stable` ของ registry ณ ตอน build · ล็อก version ด้วย `MANAGER_VERSION=1.1.0` |
| ได้ตัวล่าสุด | build ใหม่: `docker compose build --no-cache machine` (Docker cache ชั้นที่โหลด manager ไว้) |
| อัปเดตจาก center | ทำไม่ได้: `manager.update` ต้องมีโฟลเดอร์ที่ `install-manager.sh` สร้าง · เปลี่ยน version = build image ใหม่ |
| ตัวตน | อยู่ใน volume `manager-data` (แบบ 3: `manager-data-local`) · enroll ครั้งแรกด้วย `TOKEN` แล้วไม่ต้องใส่อีก · ล้างแล้วเริ่มใหม่: `docker compose down -v` |
| ลายเซ็น | image ตรวจ sha256 และลายเซ็น Ed25519 ของ manager ด้วย `machine/verify` (stdlib ล้วน) · ไม่ผ่าน = build ล้ม |

## เลิกใช้

```bash
docker compose --profile local down -v     # หยุดและลบ container กับ volume ทุกแบบ
rm -rf data .env                           # registry ในเครื่อง, key ทิ้ง, ข้อมูลของ center ตัวอย่าง
```

เอกสารทั้งระบบ: `docs/` ที่โฟลเดอร์รวม (หน้า "ทดสอบ" ของหมวด Development)
