// q2ctl sends one Q2SPOT/1 request to a card app's socket, as Q2 Pod's page does, and prints the
// reply: for driving the app on a computer.
//
//	q2ctl -sock /tmp/x/q2-lma.browse.sock menu
//	q2ctl -sock … -watch 2s menu      (asks again every 2 s, as a "refresh" list is)
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"q2-lma/internal/q2spot"
)

func main() {
	sock := flag.String("sock", "/tmp/q2-lma.browse.sock", "the app's browse socket")
	watch := flag.Duration("watch", 0, "ask again this often (0: once)")
	raw := flag.Bool("raw", false, "print the reply's bytes as they came, unparsed")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: q2ctl [-sock PATH] [-watch DUR] OP [ARG…]")
		os.Exit(2)
	}
	if *raw {
		b, err := q2spot.AskRaw(*sock, "1", flag.Arg(0), flag.Args()[1:]...)
		fmt.Printf("%q\n", b)
		if err != nil {
			fmt.Fprintln(os.Stderr, "q2ctl:", err)
			os.Exit(1)
		}
		return
	}
	for id := 1; ; id++ {
		r, err := q2spot.Ask(*sock, strconv.Itoa(id), flag.Arg(0), flag.Args()[1:]...)
		if err != nil {
			fmt.Fprintln(os.Stderr, "q2ctl:", err)
			os.Exit(1)
		}
		fmt.Printf("== %s  status=%s message=%q cursor=%q\n", time.Now().Format("15:04:05"), r.Status, r.Message, r.Cursor)
		for i, row := range r.Rows {
			fmt.Printf("%2d %-8s %-28s %s", i, row.Kind, row.Title, row.Subtitle)
			if row.URI != "" || row.Context != "" || row.Position != "" {
				fmt.Printf("   [uri=%q ctx=%q pos=%q]", row.URI, row.Context, row.Position)
			}
			fmt.Println()
		}
		if *watch == 0 || (r.Message != q2spot.Refresh && id > 1) {
			return
		}
		time.Sleep(*watch)
	}
}
