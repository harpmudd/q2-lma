#!/usr/bin/env python3
"""Run launcher.sh (the card's run) on a Linux host with a fake app: a second start while it runs
starts nothing; a failed run is kept in /tmp/q2-lma.crash and the app starts again; a TERM to the
loop's pid stops the app and tidies /tmp. Usage: test_launcher.py (uses /tmp/q2-lma*, as the Q2)."""
import os
import pathlib
import signal
import subprocess
import tempfile
import time

HERE = pathlib.Path(__file__).parent
PID = pathlib.Path('/tmp/q2-lma')


def wait(ok, what, secs=15):
    end = time.time() + secs
    while not ok():
        if time.time() > end:
            raise SystemExit(f'test_launcher.py: timed out waiting for {what}')
        time.sleep(0.05)


def main():
    for p in pathlib.Path('/tmp').glob('q2-lma*'):
        p.unlink()
    with tempfile.TemporaryDirectory() as d:
        app = pathlib.Path(d) / '.lma'
        app.mkdir()
        (app / 'run').write_bytes((HERE / 'launcher.sh').read_bytes())
        runs = pathlib.Path(d) / 'runs'
        # the fake app: fails its first run, then runs until a TERM
        (app / 'q2-lma').write_text(f'''#!/bin/sh
echo "$*" >> {runs}
if [ ! -f {d}/failed ]; then touch {d}/failed; echo boom >&2; exit 3; fi
trap 'echo term >> {runs}; exit 0' TERM
while :; do sleep 0.1; done
''')
        (app / 'q2-lma').chmod(0o755)
        subprocess.run(['sh', str(app / 'run')], check=True)
        wait(PID.exists, 'the pid file')
        loop = int(PID.read_text())
        subprocess.run(['sh', str(app / 'run')], check=True)
        assert int(PID.read_text()) == loop, 'a second start replaced the loop'
        wait(lambda: pathlib.Path('/tmp/q2-lma.crash').exists(), 'the crash file')
        crash = pathlib.Path('/tmp/q2-lma.crash').read_text()
        assert 'boom' in crash and 'exit 3' in crash, crash
        wait(lambda: runs.exists() and len(runs.read_text().splitlines()) == 2, 'the restart', 20)
        assert runs.read_text().splitlines()[1] == f'-dir {app}', runs.read_text()
        os.kill(loop, signal.SIGTERM)
        wait(lambda: not PID.exists(), 'the loop to end')
        assert 'term' in runs.read_text()
        for f in ('q2-lma.bin', 'q2-lma.run', 'q2-lma.err'):
            assert not pathlib.Path('/tmp', f).exists(), f
    print('test_launcher.py: ok')


if __name__ == '__main__':
    main()
