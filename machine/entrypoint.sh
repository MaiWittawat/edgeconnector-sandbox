#!/bin/sh
# entrypoint.sh = ให้ container มี machine-id ที่อยู่ได้นานเท่าตัวตน แล้วรัน manager
#
# center ผูกตัวตนกับ fingerprint ของเครื่อง ซึ่งอ่านจาก /etc/machine-id (ไม่มีก็ใช้ hostname)
# image alpine ไม่มีไฟล์นั้น hostname ของ container จึงถูกใช้แทน และเปลี่ยนทุกครั้งที่สร้าง container ใหม่
# (build image ใหม่, up --force-recreate) ขณะที่ identity.json ยังอยู่ใน volume
# center เลยเห็นเป็น identity ที่ถูกก๊อปไปเครื่องอื่นแล้ว suspend ("machine fingerprint mismatch")
#
# จึงเก็บ machine-id ไว้ใน /data volume เดียวกับตัวตน (/etc/machine-id ใน image เป็น symlink มาที่นี่)
# ลบ volume = ได้ทั้งตัวตนใหม่และเครื่องใหม่ เหมือนลงเครื่องใหม่จริง
set -eu

id_file="${CC_DATA:-/data}/machine-id"
if [ ! -s "$id_file" ]; then
	# 32 hex ตัวเล็ก รูปแบบเดียวกับ /etc/machine-id ของ systemd · เขียนไฟล์ชั่วคราวแล้ว mv กันไฟล์ครึ่ง ๆ ถ้าถูกฆ่ากลางทาง
	od -An -N16 -tx1 /dev/urandom | tr -d ' \n' > "$id_file.tmp"
	mv "$id_file.tmp" "$id_file"
fi

exec manager "$@"
