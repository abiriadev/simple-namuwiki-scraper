package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
	"github.com/oklog/ulid/v2"
	slogmulti "github.com/samber/slog-multi"
)

const pD = 0755
const pF = 0644

var wRegex = regexp.MustCompile("^/w/(.*)")

func main() {

	logfName := fmt.Sprintf(
		"logs/%s_%s.log.ndjson",
		ulid.Make().String(),
		time.Now().Format("1-2_3PM:4"),
	)
	os.MkdirAll(filepath.Dir(logfName), pD)

	logf, err := os.OpenFile(logfName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, pF)
	if err != nil {
		panic(err)
	}

	slog.SetDefault(
		slog.New(slogmulti.Fanout(
			log.NewWithOptions(os.Stdout, log.Options{
				Level:           log.DebugLevel,
				ReportTimestamp: true,
				TimeFormat:      time.Kitchen,
			}),

			slog.NewJSONHandler(logf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}),
		)),
	)

	domain := "namu.wiki"
	targetUrl := "https://" + domain

	c := colly.NewCollector(
		colly.AllowedDomains(domain),
		// colly.URLFilters(w),
		colly.MaxDepth(0),
		colly.CacheDir("./.cache"),
	)

	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")

		if wRegex.MatchString(link) {
			text := e.Text
			rawWord := wRegex.FindStringSubmatch(link)[1]
			word, _ := url.PathUnescape(rawWord)

			g := e.Request.Ctx.GetAny("graph").(*[]struct {
				w     string
				alias string
			})
			*g = append(*g, struct {
				w     string
				alias string
			}{word, text})

			slog.Debug("link", "word", word, "alias", text)

			ctx := colly.NewContext()
			ctx.Put("w", word)
			newg := make([]struct {
				w     string
				alias string
			}, 0)
			ctx.Put("graph", &newg)

			c.Request("GET", targetUrl+link, nil, ctx, nil)
		}
	})

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Delay:       time.Second,
		Parallelism: 1,
	})

	c.OnRequest(func(r *colly.Request) {
		slog.Info("visiting", "w", r.Ctx.Get("w"))
	})

	c.OnResponse(func(r *colly.Response) {
		w := r.Ctx.Get("w")
		path := "w/" + w + ".html"
		os.MkdirAll(filepath.Dir(path), pD)
		err := r.Save(path)
		if err != nil {
			panic(err)
		}
	})

	c.OnScraped(func(r *colly.Response) {
		g := r.Ctx.GetAny("graph").(*[]struct {
			w     string
			alias string
		})

		var buf strings.Builder
		for _, edge := range *g {
			buf.WriteString(edge.w)
			buf.WriteString("-->")
			buf.WriteString(edge.alias)
			buf.WriteRune('\n')
		}

		w := r.Ctx.Get("w")
		path := "w/" + w + ".link"
		os.WriteFile(path, []byte(buf.String()), 0644)
	})

	c.OnError(func(r *colly.Response, err error) {
		slog.Error(err.Error())
	})

	ctx := colly.NewContext()
	ctx.Put("w", "나무위키:대문")
	g := make([]struct {
		w     string
		alias string
	}, 0)
	ctx.Put("graph", &g)
	err = c.Request("GET", targetUrl, nil, ctx, nil)
	if err != nil {
		panic(err)
	}
}
