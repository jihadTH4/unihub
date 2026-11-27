#!/usr/bin/env python3
"""Builds rsrc_windows_amd64.syso (icon + version info + manifest) for the Go linker.
Needs only the Python standard library. Usage: python3 tools/winres.py [version]"""
import struct, sys, os

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
version = (sys.argv[1] if len(sys.argv) > 1 else "0.0.0").lstrip("v")
nums = [int(x) if x.isdigit() else 0 for x in (version.split("-")[0].split(".") + ["0"] * 4)[:4]]

def read(p):
    with open(os.path.join(ROOT, p), "rb") as f:
        return f.read()

# ---------- icons ----------
sizes = [256, 128, 64, 48, 32, 24, 16]
pngs = [read(f"assets/i{s}.png") for s in sizes]
grp = struct.pack("<HHH", 0, 1, len(pngs))
for i, (s, data) in enumerate(zip(sizes, pngs)):
    b = 0 if s >= 256 else s
    grp += struct.pack("<BBBBHHIH", b, b, 0, 0, 1, 32, len(data), i + 1)

# also write a standard .ico file for shortcuts/web
ico = struct.pack("<HHH", 0, 1, len(pngs))
off = 6 + 16 * len(pngs)
for s, data in zip(sizes, pngs):
    b = 0 if s >= 256 else s
    ico += struct.pack("<BBBBHHII", b, b, 0, 0, 1, 32, len(data), off)
    off += len(data)
ico += b"".join(pngs)
with open(os.path.join(ROOT, "assets/icon.ico"), "wb") as f:
    f.write(ico)

# ---------- version info ----------
def u16(s):
    return (s + "\0").encode("utf-16-le")

def pad4(b):
    return b + b"\0" * ((4 - len(b) % 4) % 4)

def node(key, value=b"", wtype=1, children=(), value_len=None):
    head_rest = u16(key)
    body = pad4(struct.pack("<HHH", 0, 0, 0) + head_rest)
    if value:
        body = pad4(body + value)
    for c in children:
        body = pad4(body)
        body += c
    if value_len is None:
        value_len = len(value) // 2 if wtype == 1 else len(value)
    length = len(body)
    return struct.pack("<HHH", length, value_len, wtype) + body[6:]

strings = {
    "CompanyName": "UniHub",
    "FileDescription": "منظّم الجامعة - UniHub",
    "FileVersion": version,
    "InternalName": "UniHub",
    "LegalCopyright": "UniHub",
    "OriginalFilename": "UniHub.exe",
    "ProductName": "منظّم الجامعة",
    "ProductVersion": version,
}
str_nodes = [node(k, u16(v), 1) for k, v in strings.items()]
table = node("040904B0", b"", 1, str_nodes, 0)
sfi = node("StringFileInfo", b"", 1, [table], 0)
var = node("Translation", struct.pack("<HH", 0x0409, 0x04B0), 0)
vfi = node("VarFileInfo", b"", 1, [var], 0)
ms = (nums[0] << 16) | nums[1]
ls = (nums[2] << 16) | nums[3]
fixed = struct.pack("<13I", 0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x40004, 1, 0, 0, 0)
verinfo = node("VS_VERSION_INFO", fixed, 0, [sfi, vfi], len(fixed))

manifest = b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="UniHub" version="%d.%d.%d.%d"/>
  <description>UniHub</description>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges>
    <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
  </requestedPrivileges></security></trustInfo>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1"><application>
    <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
  </application></compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings>
    <activeCodePage xmlns="http://schemas.microsoft.com/SMI/2019/WindowsSettings">UTF-8</activeCodePage>
  </windowsSettings></application>
</assembly>
""" % tuple(nums)

# resources: type -> list of (id, data)
res = {3: [(i + 1, d) for i, d in enumerate(pngs)], 14: [(1, grp)], 16: [(1, verinfo)], 24: [(1, manifest)]}
LANG = 0x0409

# ---------- resource section layout ----------
types = sorted(res)
dir_size = lambda n: 16 + 8 * n
# offsets: root dir, type dirs, name dirs, lang dirs, data entries, then data
off = dir_size(len(types))
type_dir_off = {}
for t in types:
    type_dir_off[t] = off
    off += dir_size(len(res[t]))
name_dir_off = {}
for t in types:
    for rid, _ in res[t]:
        name_dir_off[(t, rid)] = off
        off += dir_size(1)
entry_off = {}
for t in types:
    for rid, _ in res[t]:
        entry_off[(t, rid)] = off
        off += 16
data_off = {}
for t in types:
    for rid, d in res[t]:
        off = (off + 7) & ~7
        data_off[(t, rid)] = off
        off += len(d)
total = (off + 7) & ~7

sec = bytearray(total)
def put_dir(at, entries):
    struct.pack_into("<IIHHHH", sec, at, 0, 0, 0, 0, 0, len(entries))
    for i, (ident, target) in enumerate(entries):
        struct.pack_into("<II", sec, at + 16 + 8 * i, ident, target)

put_dir(0, [(t, 0x80000000 | type_dir_off[t]) for t in types])
relocs = []
for t in types:
    put_dir(type_dir_off[t], [(rid, 0x80000000 | name_dir_off[(t, rid)]) for rid, _ in res[t]])
    for rid, d in res[t]:
        put_dir(name_dir_off[(t, rid)], [(LANG, entry_off[(t, rid)])])
        e = entry_off[(t, rid)]
        struct.pack_into("<IIII", sec, e, data_off[(t, rid)], len(d), 0, 0)
        relocs.append(e)  # OffsetToData must become an RVA
        sec[data_off[(t, rid)]:data_off[(t, rid)] + len(d)] = d

# ---------- COFF object ----------
hdr_size, sh_size = 20, 40
raw_ptr = hdr_size + sh_size
reloc_ptr = raw_ptr + len(sec)
sym_ptr = reloc_ptr + 10 * len(relocs)
coff = struct.pack("<HHIIIHH", 0x8664, 1, 0, sym_ptr, 1, 0, 0)
coff += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(sec), raw_ptr, reloc_ptr, 0, len(relocs), 0, 0x40000040)
coff += bytes(sec)
for r in relocs:
    coff += struct.pack("<IIH", r, 0, 3)  # IMAGE_REL_AMD64_ADDR32NB against symbol 0
coff += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)
coff += struct.pack("<I", 4)  # empty string table
with open(os.path.join(ROOT, "rsrc_windows_amd64.syso"), "wb") as f:
    f.write(coff)
print("wrote rsrc_windows_amd64.syso", len(coff), "bytes, version", version)
