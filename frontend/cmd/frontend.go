// Command frontend demonstrates how to run the web app
package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jivesearch/jivesearch/instant/location"
	"github.com/jivesearch/jivesearch/instant/weather"

	"time"

	"github.com/abursavich/nett"
	"github.com/garyburd/redigo/redis"
	"github.com/jivesearch/jivesearch/bangs"
	"github.com/jivesearch/jivesearch/config"
	"github.com/jivesearch/jivesearch/frontend"
	"github.com/jivesearch/jivesearch/frontend/cache"
	"github.com/jivesearch/jivesearch/instant"
	"github.com/jivesearch/jivesearch/instant/discography/musicbrainz"
	"github.com/jivesearch/jivesearch/instant/parcel"
	"github.com/jivesearch/jivesearch/instant/stackoverflow"
	"github.com/jivesearch/jivesearch/instant/stock"
	"github.com/jivesearch/jivesearch/instant/wikipedia"
	"github.com/jivesearch/jivesearch/log"
	"github.com/jivesearch/jivesearch/search"
	"github.com/jivesearch/jivesearch/search/document"
	"github.com/jivesearch/jivesearch/search/vote"
	"github.com/jivesearch/jivesearch/suggest"
	"github.com/lib/pq"
	"github.com/olivere/elastic"
	"github.com/spf13/viper"
	"golang.org/x/text/language"
)

var (
	f *frontend.Frontend
)

func setup(v *viper.Viper) *http.Server {
	v.SetEnvPrefix("jivesearch")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	config.SetDefaults(v)

	if v.GetBool("debug") {
		log.Debug.SetOutput(os.Stdout)
	}

	frontend.ParseTemplates()
	f = &frontend.Frontend{
		Brand: frontend.Brand{
			Name:      v.GetString("brand.name"),
			TagLine:   v.GetString("brand.tagline"),
			Logo:      v.GetString("brand.logo"),
			SmallLogo: v.GetString("brand.small_logo"),
		},
	}

	router := f.Router(v)

	return &http.Server{
		Addr:    ":" + strconv.Itoa(v.GetInt("frontend.port")),
		Handler: http.TimeoutHandler(router, 5*time.Second, "Sorry, we took too long to get back to you"),
	}
}

func main() {
	v := viper.New()
	s := setup(v)

	// Set the backend for our core search results
	client, err := elastic.NewClient(
		elastic.SetURL(v.GetString("elasticsearch.url")),
		elastic.SetSniff(false),
	)

	if err != nil {
		panic(err)
	}

	f.Search = &search.ElasticSearch{
		ElasticSearch: &document.ElasticSearch{
			Client: client,
			Index:  v.GetString("elasticsearch.search.index"),
			Type:   v.GetString("elasticsearch.search.type"),
		},
	}

	// autocomplete & phrase suggestor
	f.Suggest = &suggest.ElasticSearch{
		Client: client,
		Index:  v.GetString("elasticsearch.query.index"),
		Type:   v.GetString("elasticsearch.query.type"),
	}

	exists, err := f.Suggest.IndexExists()
	if err != nil {
		panic(err)
	}

	if !exists {
		if err := f.Suggest.Setup(); err != nil {
			panic(err)
		}
	}

	// !bangs
	f.Bangs = bangs.New()
	f.Bangs.Suggester = &bangs.ElasticSearch{
		Client: client,
		Index:  v.GetString("elasticsearch.bangs.index"),
		Type:   v.GetString("elasticsearch.bangs.type"),
	}

	exists, err = f.Bangs.Suggester.IndexExists()
	if err != nil {
		panic(err)
	}

	if exists { // always want to recreate to add any changes/new !bangs
		if err := f.Bangs.Suggester.DeleteIndex(); err != nil {
			panic(err)
		}
	}

	if err := f.Bangs.Suggester.Setup(f.Bangs.Bangs); err != nil {
		panic(err)
	}

	// cache
	rds := &cache.Redis{
		RedisPool: &redis.Pool{
			MaxIdle:     1,
			MaxActive:   1,
			IdleTimeout: 10 * time.Second,
			Wait:        true,
			Dial: func() (redis.Conn, error) {
				cl, err := redis.Dial("tcp", fmt.Sprintf("%v:%v", v.GetString("redis.host"), v.GetString("redis.port")))
				if err != nil {
					return nil, err
				}
				return cl, err
			},
		},
	}

	defer rds.RedisPool.Close()

	f.Cache.Cacher = rds
	if err != nil {
		panic(err)
	}
	f.Cache.Instant = v.GetDuration("cache.instant")
	f.Cache.Search = v.GetDuration("cache.search")

	// The database needs to be setup beforehand.
	db, err := sql.Open("postgres",
		fmt.Sprintf(
			"user=%s password=%s host=%s database=%s sslmode=require",
			v.GetString("postgresql.user"),
			v.GetString("postgresql.password"),
			v.GetString("postgresql.host"),
			v.GetString("postgresql.database"),
		),
	)
	if err != nil {
		panic(err)
	}

	defer db.Close()
	db.SetMaxIdleConns(0)

	// Instant Answers
	httpClient := &http.Client{
		Transport: &http.Transport{
			Dial: (&nett.Dialer{
				Resolver: &nett.CacheResolver{TTL: 10 * time.Minute},
				IPFilter: nett.DualStack,
			}).Dial,
			DisableKeepAlives: true,
		},
		Timeout: 5 * time.Second,
	}

	f.GitHub = frontend.GitHub{
		HTTPClient: httpClient,
	}

	f.Instant = &instant.Instant{
		QueryVar: "q",
		DiscographyFetcher: &musicbrainz.PostgreSQL{
			DB: db,
		},
		LocationFetcher: &location.MaxMind{
			DB: v.GetString("maxmind.database"),
		},
		FedExFetcher: &parcel.FedEx{
			HTTPClient: httpClient,
			Account:    v.GetString("fedex.account"),
			Password:   v.GetString("fedex.password"),
			Key:        v.GetString("fedex.key"),
			Meter:      v.GetString("fedex.meter"),
		},
		StackOverflowFetcher: &stackoverflow.API{
			HTTPClient: httpClient,
			Key:        v.GetString("stackoverflow.key"),
		},
		StockQuoteFetcher: &stock.IEX{
			HTTPClient: httpClient,
		},
		UPSFetcher: &parcel.UPS{
			HTTPClient: httpClient,
			User:       v.GetString("ups.user"),
			Password:   v.GetString("ups.password"),
			Key:        v.GetString("ups.key"),
		},
		USPSFetcher: &parcel.USPS{
			HTTPClient: httpClient,
			User:       v.GetString("usps.user"),
			Password:   v.GetString("usps.password"),
		},
		WeatherFetcher: &weather.OpenWeatherMap{
			HTTPClient: httpClient,
			Key:        v.GetString("openweathermap.key"),
		},
		WikipediaFetcher: &wikipedia.PostgreSQL{
			DB: db,
		},
	}

	if err := f.Instant.WikipediaFetcher.Setup(); err != nil {
		log.Info.Println(err)
	}

	// Voting
	f.Vote = &vote.PostgreSQL{
		DB:    db,
		Table: v.GetString("postgresql.votes.table"),
	}

	if err := f.Vote.Setup(); err != nil {
		switch err.(type) {
		case *pq.Error:
			if err.(*pq.Error).Error() != vote.ErrScoreFnExists.Error() {
				panic(err)
			}
		default:
			panic(err)
		}
	}

	// supported languages
	supported, unsupported := languages(v)
	for _, lang := range unsupported {
		log.Info.Printf("wikipedia does not support langugage %q\n", lang)
	}

	f.Wikipedia.Matcher = language.NewMatcher(supported)

	// see notes on customizing languages in search/document/document.go
	f.Document.Languages = document.Languages(supported)
	f.Document.Matcher = language.NewMatcher(f.Document.Languages)

	log.Info.Printf("Listening at http://127.0.0.1%v", s.Addr)
	log.Info.Fatal(s.ListenAndServe())
}

func languages(cfg config.Provider) ([]language.Tag, []language.Tag) {
	supported := []language.Tag{}

	for _, l := range cfg.GetStringSlice("languages") {
		supported = append(supported, language.MustParse(l))
	}

	return wikipedia.Languages(supported)
}
