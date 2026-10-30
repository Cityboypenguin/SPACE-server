// Copy only revoked:* keys. Never copies maintenance flags, OTPs or rate limits.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type record struct {
	Key     string `json:"key"`
	Expires int64  `json:"expires_unix_ms"`
}

func main() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "Revocation transfer failed; credentials and keys are suppressed")
			os.Exit(1)
		}
	}()
	export := flag.String("export", "", "export revoked keys to a restricted file")
	imp := flag.String("import", "", "import exported keys; --apply required to write")
	apply := flag.Bool("apply", false, "write destination revocations")
	flag.Parse()
	if (*export == "") == (*imp == "") {
		panic("choose one mode")
	}
	options, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	must(err)
	client := redis.NewClient(options)
	defer client.Close()
	ctx := context.Background()
	if *export != "" {
		records := []record{}
		var cursor uint64
		for {
			keys, next, e := client.Scan(ctx, cursor, "revoked:*", 100).Result()
			must(e)
			for _, key := range keys {
				capturedAt := time.Now().UnixMilli()
				ttl, e := client.PTTL(ctx, key).Result()
				must(e)
				if ttl == -1 {
					panic("unexpected permanent revocation")
				}
				if ttl > 0 {
					records = append(records, record{key, capturedAt + ttl.Milliseconds()})
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		f, e := os.OpenFile(*export, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		must(e)
		must(json.NewEncoder(f).Encode(records))
		must(f.Sync())
		must(f.Close())
		fmt.Printf("Exported %d revocations; keep this file secret\n", len(records))
		return
	}
	b, e := os.ReadFile(*imp)
	must(e)
	var records []record
	must(json.Unmarshal(b, &records))
	n := 0
	for _, r := range records {
		if len(r.Key) < 8 || r.Key[:8] != "revoked:" {
			panic("unexpected prefix")
		}
		ttl := time.Until(time.UnixMilli(r.Expires))
		if ttl <= 0 {
			continue
		}
		n++
		if *apply {
			// Never shorten a revocation already on the destination; expiry stays absolute.
			_, e = client.Eval(ctx, `local t=redis.call('PTTL',KEYS[1]); if t == -2 or (t >= 0 and t < tonumber(ARGV[1])) then return redis.call('PSETEX',KEYS[1],ARGV[1],'1') end; return 0`, []string{r.Key}, ttl.Milliseconds()).Result()
			must(e)
		}
	}
	fmt.Printf("Unexpired revocations: %d; apply=%t\n", n, *apply)
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
