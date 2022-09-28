package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/makeworld-the-better-one/go-gemini"
)

var client *http.Client

type Twtxts struct {
	twtxts     map[string]*Twtxt
	Mentions   int64
	Tweets     int64
	BytesTotal int64
}

var twtxts Twtxts
var links []string

var ch chan string
var lock sync.RWMutex

func init() {
	client = getHTTPClient()

	startingTwtxt := "https://niplav.github.io/twtxt.txt"

	twtxts = Twtxts{
		twtxts: map[string]*Twtxt{startingTwtxt: NewTwtxt("startingTwtxt")},
	}
	//links = []string{startingTwtxt}
	ch = make(chan string, 30)
	ch <- startingTwtxt
}

func (ts Twtxts) MentionStd() float64 {
	var mean, sd float64

	mean = float64(ts.Mentions) / float64(ts.SumUsers())

	for _, v := range twtxts.twtxts {
		sd += math.Pow(float64(v.MentionsSum())-mean, 2)
	}
	sd = math.Sqrt(sd / float64(ts.SumUsers()))

	return sd
}

func (ts Twtxts) MentionAvg() float64 {
	return float64(ts.Mentions) / ts.SumUsers()
}

func (ts Twtxts) TweetsStd() float64 {
	var mean, sd float64

	mean = float64(ts.Tweets) / float64(ts.SumUsers())

	for _, v := range twtxts.twtxts {
		sd += math.Pow(float64(v.TweetsSum())-mean, 2)
	}
	sd = math.Sqrt(sd / float64(ts.SumUsers()))

	return sd
}

func (ts Twtxts) TweetsAvg() float64 {
	return float64(ts.Tweets) / ts.SumUsers()
}

func (ts Twtxts) SumUsers() float64 {
	return float64(len(ts.twtxts))
}

type Twtxt struct {
	URL             string
	Alive           bool
	Accessible      bool
	MentionsperYear map[string]int
	TwtsPerYear     map[string]int
	Interacting     []string
}

func NewTwtxt(url string) *Twtxt {
	return &Twtxt{
		URL:             url,
		MentionsperYear: map[string]int{},
		TwtsPerYear:     map[string]int{},
		Interacting:     []string{},
	}
}

func (t Twtxt) MentionsSum() float64 {
	var sum int
	for _, v := range t.MentionsperYear {
		sum += v
	}

	return float64(sum)
}

func (t Twtxt) TweetsSum() float64 {
	var sum int
	for _, v := range t.TwtsPerYear {
		sum += v
	}

	return float64(sum)
}

func main() {

	home, _ := os.UserHomeDir()

	start := time.Now()

	var wg sync.WaitGroup
	go func() {
		wg.Wait()
		close(ch)
	}()

	var index int64 = 0
	for url := range ch {
		wg.Add(1)
		links = append(links, url)
		fmt.Println(url)
		go func(url string) {
			if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
				fetchHttp(url)
			}

			if strings.HasPrefix(url, "gemini://") {
				fetchGemini(url)
			}

			if strings.HasPrefix(url, "gopher://") {
				//fetchGopher(url)
			}

			atomic.AddInt64(&index, 1)
			if index%100 == 0 {
				fmt.Printf("[%d/%d] status\n", index, len(links))
			}
			wg.Done()
		}(url)
	}

	accessible, err := os.Create(home + "/accessible.csv")
	if err != nil {
		return
	}
	defer accessible.Close()
	wAcc := bufio.NewWriter(accessible)

	active, err := os.Create(home + "/active.csv")
	if err != nil {
		return
	}
	defer active.Close()
	wAct := bufio.NewWriter(active)

	rank, err := os.Create(home + "/rank.csv")
	if err != nil {
		return
	}
	defer rank.Close()
	wRank := bufio.NewWriter(rank)
	avgM := twtxts.MentionAvg()
	stdM := twtxts.MentionStd()

	for k, v := range twtxts.twtxts {

		if v.Alive {
			fmt.Fprintf(wAct, "%s\n", k)
		}

		if v.Accessible {
			fmt.Fprintf(wAcc, "%s\n", k)
			fmt.Fprintf(wRank, "%s;%v\n", k, (v.MentionsSum()-avgM)/stdM)
		}

	}

	wAcc.Flush()
	wAct.Flush()
	wRank.Flush()

	fmt.Printf("we went through %v links, downloaded %v Mb in %v\n", twtxts.SumUsers(), twtxts.BytesTotal/1024/1024, time.Since(start))

}

func fetchHttp(url string) (map[string]*Twtxt, []string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil
	}

	req.Header.Set("User-Agent", "twx/0.1.0 (+https://github.com/tkanos/we-are-twtxt; @we-are-twtxt-crawler)")

	res, err := client.Do(req)
	if err != nil {
		return nil, nil
	}

	if res.StatusCode != 200 {
		return nil, nil
	} else {
		twtxts.twtxts[url].Accessible = true
	}

	body := res.Body
	defer body.Close()

	b, err := io.ReadAll(body)
	if err != nil {
		return nil, nil
	}
	atomic.AddInt64(&twtxts.BytesTotal, int64(len(b)))

	parseBody(url, b)

	return twtxts.twtxts, links
}

func fetchGemini(url string) (map[string]*Twtxt, []string) {
	res, err := gemini.Fetch(url)
	if err != nil {
		return nil, nil
	}
	defer res.Body.Close()

	if res.Status > 200 {
		return nil, nil
	}
	twtxts.twtxts[url].Accessible = true

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, nil
	}
	atomic.AddInt64(&twtxts.BytesTotal, int64(len(body)))

	parseBody(url, body)

	return twtxts.twtxts, links

}

func parseBody(link string, body []byte) (map[string]*Twtxt, []string) {
	alive := false
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		newlink, year, err := parseLine(line)
		if !alive && year == "2022" {
			alive = true
		}

		if err == nil {

			lock.Lock()
			twtxts.twtxts[link].TwtsPerYear[year]++
			lock.Unlock()
			atomic.AddInt64(&twtxts.Tweets, 1)
		}
		if err == nil && newlink != "" {
			lock.RLock()
			_, ok := twtxts.twtxts[newlink]
			lock.RUnlock()

			if !ok {
				twtxt := NewTwtxt(newlink)
				twtxt.MentionsperYear[year] = 1

				lock.Lock()
				twtxts.twtxts[newlink] = twtxt
				lock.Unlock()

				atomic.AddInt64(&twtxts.Mentions, 1)

				ch <- newlink
			} else {
				if alive {
					twtxts.twtxts[link].Alive = true
					twtxts.twtxts[link].Interacting = append(twtxts.twtxts[link].Interacting, newlink)
				}

				if newlink != "" {
					lock.Lock()
					twtxts.twtxts[newlink].MentionsperYear[year]++
					lock.Unlock()
					atomic.AddInt64(&twtxts.Mentions, 1)
				}
			}
		}

	}

	return twtxts.twtxts, links
}

var re = regexp.MustCompile(`^(([0-9]{4})\-[0-9]{2}\-[0-9]{2}){0,1}.*((http|gemini|gopher)[^ ]+\/twtxt\.txt).*$`)

//var re = regexp.MustCompile(`^.*((http|gemini|gopher).+[a-z|A-Z]/twtxt\.txt).*$`)

func parseLine(line string) (link string, year string, err error) {
	if line == "" {
		return "", "", nil
	}

	groups := re.FindStringSubmatch(line)
	if groups == nil || groups[0] == "" {
		return "", "", nil
	}

	if !strings.Contains(groups[3], "feeds.twtxt.net") && !strings.Contains(groups[3], "feeds.twtxt.cc") {
		return groups[3], groups[2], nil
	}

	return "", "", nil
}
