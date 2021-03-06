package instant

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jivesearch/jivesearch/instant/stock"
	"golang.org/x/text/language"
)

// StockQuote is an instant answer
type StockQuote struct {
	Fetcher stock.Fetcher
	Answer
}

func (s *StockQuote) setQuery(r *http.Request, qv string) answerer {
	s.Answer.setQuery(r, qv)
	return s
}

func (s *StockQuote) setUserAgent(r *http.Request) answerer {
	return s
}

func (s *StockQuote) setLanguage(lang language.Tag) answerer {
	s.language = lang
	return s
}

func (s *StockQuote) setType() answerer {
	s.Type = "stock quote"
	return s
}

func (s *StockQuote) setRegex() answerer {
	triggers := []string{
		"quote", "stock", "stock quote",
	}

	for i, tr := range triggers {
		triggers[i] = tr + "[s]?"
	}

	t := strings.Join(triggers, "|")
	ticker := `^[\$]?[a-zA-Z]{1,5}[\.]?[a-zA-Z]?` // e.g. BRK.A

	s.regex = append(s.regex, regexp.MustCompile(fmt.Sprintf(`^(?P<trigger>%s)?\s?(?P<remainder>%s)$`, t, ticker)))
	s.regex = append(s.regex, regexp.MustCompile(fmt.Sprintf(`^(?P<remainder>%s)\s(?P<trigger>%s)?$`, ticker, t)))

	return s
}

func (s *StockQuote) solve(r *http.Request) answerer {
	ticker := strings.ToUpper(strings.Replace(s.remainder, "$", "", -1))

	resp, err := s.Fetcher.Fetch(ticker)
	if err != nil {
		s.Err = err
		return s
	}

	resp = resp.SortHistorical()

	s.Data.Solution = resp
	return s
}

func (s *StockQuote) setCache() answerer {
	s.Cache = true
	return s
}

func (s *StockQuote) tests() []test {
	typ := "stock quote"

	location, _ := time.LoadLocation("America/New_York")

	tests := []test{
		{
			query: "AAPL quote",
			expected: []Data{
				{
					Type:      typ,
					Triggered: true,
					Solution: &stock.Quote{
						Ticker:   "AAPL",
						Name:     "Apple Inc.",
						Exchange: stock.NASDAQ,
						Last: stock.Last{
							Price:         171.42,
							Time:          time.Unix(1522090355062/1000, 0).In(location),
							Change:        6.48,
							ChangePercent: 0.03929,
						},
						History: []stock.EOD{
							{Date: time.Date(2013, 3, 26, 0, 0, 0, 0, time.UTC), Open: 60.5276, Close: 59.9679, High: 60.5797, Low: 59.8891, Volume: 73428208},
							{Date: time.Date(2013, 3, 27, 0, 0, 0, 0, time.UTC), Open: 59.3599, Close: 58.7903, High: 59.4041, Low: 58.6147, Volume: 81854409},
						},
						Provider: stock.IEXProvider,
					},
					Cache: true,
				},
			},
		},
		{
			query: "brk.a", // test for lowercase and has "."
			expected: []Data{
				{
					Type:      typ,
					Triggered: true,
					Solution: &stock.Quote{
						Ticker:   "BRK.A",
						Name:     "Berkshire Hathaway",
						Exchange: stock.NYSE,
						Last: stock.Last{
							Price:         171.42,
							Time:          time.Unix(1522090355062/1000, 0).In(location),
							Change:        6.48,
							ChangePercent: 0.03929,
						},
						History: []stock.EOD{
							{Date: time.Date(2013, 3, 26, 0, 0, 0, 0, time.UTC), Open: 60.5276, Close: 59.9679, High: 60.5797, Low: 59.8891, Volume: 73428208},
							{Date: time.Date(2013, 3, 27, 0, 0, 0, 0, time.UTC), Open: 59.3599, Close: 58.7903, High: 59.4041, Low: 58.6147, Volume: 81854409},
						},
						Provider: stock.IEXProvider,
					},
					Cache: true,
				},
			},
		},
	}

	return tests
}
