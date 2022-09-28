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

	"github.com/makeworld-the-better-one/go-gemini"
)

var index = 0

var client *http.Client

type Twtxts struct {
	twtxts     map[string]*Twtxt
	Mentions   int
	Tweets     int
	BytesTotal int
}

var twtxts Twtxts
var links []string

func init() {
	client = getHTTPClient()

	startingTwtxt := "https://niplav.github.io/twtxt.txt"

	twtxts = Twtxts{
		twtxts: map[string]*Twtxt{startingTwtxt: NewTwtxt(startingTwtxt)},
	}
	links = []string{startingTwtxt}
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

	index = 0
	for true {
		if len(links) > index {
			url := links[index]
			fmt.Println(url)
			if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
				fetchHttp(url)
			}

			if strings.HasPrefix(url, "gemini://") {
				fetchGemini(url)
			}

			if strings.HasPrefix(url, "gopher://") {
				//fetchGopher(url)
			}

			index++
			if index%100 == 0 {
				fmt.Printf("[%d/%d] status \n", index, len(links))
			}
		} else {
			break
		}
	}

	accessible, err := os.Create(home + "/accesssible.csv")
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
	avgT := twtxts.TweetsAvg()
	stdT := twtxts.TweetsStd()

	for k, v := range twtxts.twtxts {
		fmt.Fprintf(wAcc, "%s\n", k)
		if v.Alive {
			fmt.Fprintf(wAct, "%s\n", k)
		}
		fmt.Fprintf(wRank, "%s;%v;%v;%v;%v\n", k, (v.MentionsSum()-avgM)/stdM, v.MentionsSum()-avgM, (v.TweetsSum()-avgT)/stdT, ((v.MentionsSum()-avgM)/stdM)*((v.TweetsSum()-avgT)/stdT))
	}

	wAcc.Flush()
	wAct.Flush()
	wRank.Flush()

	fmt.Printf("we went through %v links, downloaded %v Mb in %v\n", twtxts.SumUsers(), twtxts.BytesTotal/1024/1024)

}

func fetchHttp(url string) (map[string]*Twtxt, []string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil
	}

	req.Header.Set("User-Agent", fmt.Sprint("twx/{0.1.0 (+crawler; @https://github.com/tkanos/we-are-twtxt)"))

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

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, nil
	} else {
		twtxts.twtxts[url].Accessible = true
	}

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
		if err != nil {
			twtxts.twtxts[link].TwtsPerYear[year]++
			twtxts.Tweets++
		}
		if err == nil && newlink != "" {
			if _, ok := twtxts.twtxts[newlink]; !ok {
				twtxt := NewTwtxt(newlink)
				twtxt.Alive = alive
				twtxt.Accessible = true
				twtxt.MentionsperYear[year] = 1
				twtxts.twtxts[newlink] = twtxt

				twtxts.Mentions++

				links = append(links, newlink)
			} else {
				if alive {
					twtxts.twtxts[link].Alive = true
					twtxts.twtxts[link].Interacting = append(twtxts.twtxts[link].Interacting, newlink)
				}

				if newlink != "" {
					twtxts.twtxts[newlink].MentionsperYear[year]++
					twtxts.Mentions++
				}
			}

		}

	}

	return twtxts.twtxts, links
}

var re = regexp.MustCompile(`^(([0-9]{4})\-[0-9]{2}\-[0-9]{2}){0,1}.*((http|gemini|gopher).+[a-z|A-Z]/twtxt\.txt).*$`)

//var re = regexp.MustCompile(`^.*((http|gemini|gopher).+[a-z|A-Z]/twtxt\.txt).*$`)

func parseLine(line string) (link string, year string, err error) {
	if line == "" {
		return "", "", nil
	}

	groups := re.FindStringSubmatch(line)
	if groups == nil || groups[0] == "" {
		return "", "", nil
	}

	if !strings.Contains(groups[3], "feeds.twtxt.net") {
		return groups[3], groups[2], nil
	}

	return "", "", nil
}
