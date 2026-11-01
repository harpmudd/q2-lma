#!/usr/bin/env python3
"""Check a staged card folder for the Q2 and zip it as a release ships it.

    package.py STAGED_DIR ZIP

STAGED_DIR is OUT/.lma: the app q2-lma, its launcher run, its manifest q2app, and optionally its
Streaming icons icon.png (52 px) and icon-40.png (40 px). The app must be a static 32-bit
little-endian MIPS executable with soft-float ABI flags (GOMIPS=softfloat). The Q2's kernel also
requires the nan2008 ELF flag, which Go doesn't set, so it is set here. run and q2app must have LF
line endings: a CRLF launcher fails on the Q2 with nothing on screen but "isn't running". Entries
are dated 2026-01-01, so two builds of one commit give the same zip, and the app goes last, so an
interrupted unzip never leaves a new app beside old files."""
import pathlib
import struct
import sys
import zipfile

EF_MIPS_NAN2008 = 0x400
PT_INTERP, PT_MIPS_ABIFLAGS = 3, 0x70000003
MIPS_FP_SOFT = 3
STAMP = (2026, 1, 1, 0, 0, 0)


def fail(msg):
    raise SystemExit(f'package.py: {msg}')


def mark_q2(data):
    if data[:7] != b'\x7fELF\x01\x01\x01':
        fail('not a 32-bit little-endian ELF')
    e_type, e_machine = struct.unpack_from('<HH', data, 16)
    e_phoff, = struct.unpack_from('<I', data, 28)
    e_flags, = struct.unpack_from('<I', data, 36)
    e_phentsize, e_phnum = struct.unpack_from('<HH', data, 42)
    if e_type != 2 or e_machine != 8:
        fail('not a MIPS executable')
    fp = None
    for i in range(e_phnum):
        p_type, p_offset = struct.unpack_from('<II', data, e_phoff + i * e_phentsize)
        if p_type == PT_INTERP:
            fail('dynamically linked: build with CGO_ENABLED=0')
        if p_type == PT_MIPS_ABIFLAGS:
            fp = data[p_offset + 7]
    if fp != MIPS_FP_SOFT:
        fail(f'FP ABI {fp}, not soft float: build with GOMIPS=softfloat')
    struct.pack_into('<I', data, 36, e_flags | EF_MIPS_NAN2008)
    return data


def main():
    if len(sys.argv) != 3:
        fail('usage: package.py STAGED_DIR ZIP')
    staged, out = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
    folder = staged.name
    app = staged / ('q2-' + folder.lstrip('.'))
    app.write_bytes(mark_q2(bytearray(app.read_bytes())))
    for name in ('run', 'q2app'):
        text = (staged / name).read_bytes()
        if b'\r' in text:
            fail(f'{name} has CR line endings')
    if not (staged / 'run').read_bytes().startswith(b'#!/bin/sh\n'):
        fail('run does not start with #!/bin/sh')
    names = [n for n in ('q2app', 'icon.png', 'icon-40.png', 'run') if (staged / n).exists()]
    with zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED) as z:
        for name in names + [app.name]:
            info = zipfile.ZipInfo(f'{folder}/{name}', STAMP)
            info.external_attr = (0o100000 | (0o755 if name in ('run', app.name) else 0o644)) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, (staged / name).read_bytes())
    print(out)


if __name__ == '__main__':
    main()
