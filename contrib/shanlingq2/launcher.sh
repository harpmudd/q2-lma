#!/bin/sh
# Installed as /mnt/mmc/.lma/run. Q2 Pod runs it from Streaming's row (a card app: q2-pod's
# patch/addon.c). The app and this script are copied to /tmp and run from there, and the card's
# copy is left at once (exec), so nothing keeps the card busy and USB storage can always take it.
# A background loop restarts the app after a failure, waiting 5 s and doubling to a minute; an exit
# of 0 (quit from the Q2, or idle) ends it. A failed run's stderr and exit code are kept in
# /tmp/q2-lma.crash, which the next start copies into .lma/log.txt. Q2 Pod stops the app with a
# TERM to the loop's pid in /tmp/q2-lma; an app still there 2 s later is killed, inside the 3 s
# the firmware waits.
pid=/tmp/q2-lma
if [ "$1" != --loop ]; then
    [ -f $pid ] && kill -0 "$(cat $pid)" 2>/dev/null && exit 0
    dir=$(cd "$(dirname "$0")" && pwd)
    cp -f "$dir/q2-lma" /tmp/q2-lma.bin && chmod 755 /tmp/q2-lma.bin || exit 1
    cp -f "$0" /tmp/q2-lma.run || exit 1
    cd /tmp
    exec /bin/sh /tmp/q2-lma.run --loop "$dir"
fi
dir=$2
(
    child=
    trap 'if [ -n "$child" ]; then kill -TERM $child 2>/dev/null; (sleep 2; kill -KILL $child 2>/dev/null) & w=$!; wait $child; kill $w 2>/dev/null; fi; rm -f $pid /tmp/q2-lma.bin /tmp/q2-lma.run /tmp/q2-lma.err; exit 0' TERM INT
    delay=5
    while :; do
        /tmp/q2-lma.bin -dir "$dir" 2>/tmp/q2-lma.err &
        child=$!
        wait $child
        code=$?
        child=
        [ $code = 0 ] && break
        { cat /tmp/q2-lma.err; echo "exit $code"; } >/tmp/q2-lma.crash
        sleep $delay &
        wait $! # a TERM interrupts a wait, not a foreground sleep
        [ $delay -lt 60 ] && delay=$((delay * 2))
    done
    rm -f $pid /tmp/q2-lma.bin /tmp/q2-lma.run /tmp/q2-lma.err
) </dev/null >/dev/null 2>&1 &
echo $! >$pid
