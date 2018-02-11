// Package bangs provides functionality to query other websites
package bangs

import (
	"strings"

	"golang.org/x/text/language"
)

// Bangs holds a map of !bangs
type Bangs struct {
	Bangs []Bang
	Suggester
}

// Bang holds a single !bang
type Bang struct {
	Name            string            `json:"name"`
	Triggers        []string          `json:"triggers"`
	Regions         map[string]string `json:"regions"`
	Transformations []transformation  `json:"-"`
}

// Suggester is a !bangs suggester/autocomplete
type Suggester interface {
	IndexExists() (bool, error)
	DeleteIndex() error
	Setup([]Bang) error
	suggest(term string, size int) (Results, error)
}

// Results are the results of an autocomplete query
type Results struct { // remember top-level arrays = no-no in javascript/json
	Suggestions []Suggestion `json:"suggestions"`
}

// Suggestion is an individual !bang autocomplete suggestion
type Suggestion struct {
	Trigger string `json:"trigger"`
	Name    string `json:"name"`
}

const def = "default"

// Suggest is an autocomplete for !bangs
func (b *Bangs) Suggest(term string, size int) (Results, error) {
	res, err := b.Suggester.suggest(term, size)
	if err != nil {
		return res, err
	}

	// fill in the rest of the suggestion
	for i, s := range res.Suggestions {
		for _, bng := range b.Bangs {
			for _, trigger := range bng.Triggers {
				if trigger == s.Trigger {
					s.Name = bng.Name
					res.Suggestions[i] = s
				}
			}
		}
	}

	return res, err
}

// Detect lets us know if we have a !bang match.
func (b *Bangs) Detect(q string, region language.Region, l language.Tag) (string, bool) {
	fields := strings.Fields(q)

	for i, field := range fields {
		if field == "!" || (!strings.HasPrefix(field, "!") && !strings.HasSuffix(field, "!")) {
			continue
		}

		k := strings.ToLower(strings.Trim(field, "!"))
		for _, bng := range b.Bangs {
			if triggered := trigger(k, bng.Triggers); !triggered {
				continue
			}

			remainder := strings.Join(append(fields[:i], fields[i+1:]...), " ")

			for _, fn := range bng.Transformations {
				remainder = fn(remainder)
			}

			for _, reg := range []string{strings.ToLower(region.String()), def} { // use default region if no region specified
				if u, ok := bng.Regions[reg]; ok {
					u = strings.Replace(u, "{{{term}}}", remainder, -1)
					return strings.Replace(u, "{{{lang}}}", l.String(), -1), true
				}
			}
		}
	}
	return "", false
}

type transformation func(string) string

// Returns the canonical version of a Wikipedia title.
// "bob maRLey" -> "Bob_Marley"
// Some regional queries will be incorrect: https://es.wikipedia.org/wiki/De_la_Tierra_a_la_Luna
var wikipediaCanonical = func(q string) string {
	return strings.Replace(strings.Title(strings.ToLower(q)), " ", "_", -1)
}

func trigger(k string, triggers []string) bool {
	for _, trigger := range triggers {
		if k == trigger {
			return true
		}
	}
	return false
}

// New creates a pointer with the default !bangs.
// Use default url unless a region is provided.
// Region: US, Language: French !a ---> Amazon.com
// Region: France, Language: English !a ---> Amazon.fr
// !afr ---> Amazon.fr
// Note: Some !bangs don't respect the language passed in or
// may not support it (eg they may support pt but not pt-BR)
//
// TODO: Allow overrides...perhaps add a method or use a config.
// Note: If we end up using viper for this don't use "SetDefault"
// as overriding one !bang will replace ALL !bangs. Instead, use "Set".
func New() *Bangs {
	// Not sure about the structure here...slice of Bangs makes it easy to add bangs
	// Would like to add autocomplete feature so that people can find !bangs easier.
	b := &Bangs{}
	b.Bangs = []Bang{
		{
			"Amazon", []string{"a", "amazon"},
			map[string]string{
				def:  "https://www.amazon.com/s/ref=nb_sb_noss?url=search-alias%3Daps&field-keywords={{{term}}}",
				"ca": "https://www.amazon.ca/s/ref=nb_sb_noss?url=search-alias%3Daps&field-keywords={{{term}}}",
				"fr": "https://www.amazon.fr/s/ref=nb_sb_noss?url=search-alias%3Daps&field-keywords={{{term}}}",
				"uk": "https://www.amazon.co.uk/s/ref=nb_sb_noss?url=search-alias%3Daps&field-keywords={{{term}}}",
			},
			[]transformation{},
		},
		{
			"Bing", []string{"b", "bing"},
			map[string]string{
				def: "https://www.bing.com/search?q={{{term}}}",
			},
			[]transformation{},
		},
		{
			"GitHub", []string{"gh", "git", "github"},
			map[string]string{
				def: "https://github.com/search?q={{{term}}}&type=Everything&repo=&langOverride=&start_value=1",
			},
			[]transformation{},
		},
		{
			"Google", []string{"g", "google"},
			map[string]string{
				def:  "https://encrypted.google.com/search?hl={{{lang}}}&q={{{term}}}",
				"ca": "https://www.google.ca/search?q={{{term}}}",
				"fr": "https://www.google.fr/search?hl={{{lang}}}&q={{{term}}}",
				"ru": "https://www.google.ru/search?hl={{{lang}}}&q={{{term}}}",
			},
			[]transformation{},
		},
		{
			"Google France", []string{"gfr", "googlefr"},
			map[string]string{
				def: "https://www.google.fr/search?hl={{{lang}}}&q={{{term}}}",
			},
			[]transformation{},
		},
		{
			"Google Images", []string{"gi"},
			map[string]string{
				def: "https://www.google.com/search?q={{{term}}}&source=lnms&tbm=isch",
			},
			[]transformation{},
		},
		{
			"Google Russia", []string{"gru", "googleru"},
			map[string]string{
				def: "https://www.google.ru/search?hl={{{lang}}}&q={{{term}}}",
			},
			[]transformation{},
		},
		{
			"Reddit", []string{"reddit"},
			map[string]string{
				def: "https://www.reddit.com/search?q={{{term}}}&restrict_sr=&sort=relevance&t=all",
			},
			[]transformation{},
		},
		{
			"Stack Overflow", []string{"so", "stackoverflow"},
			map[string]string{
				def: "https://stackoverflow.com/search?q={{{term}}}",
			},
			[]transformation{},
		},
		{
			// I think these need to be mapped to languages, not regions...
			"Wikipedia", []string{"w", "wikipedia"},
			map[string]string{
				def:  "https://en.wikipedia.org/wiki/{{{term}}}",
				"es": "https://es.wikipedia.org/wiki/{{{term}}}",
				"de": "https://de.wikipedia.org/wiki/{{{term}}}",
				"fr": "https://fr.wikipedia.org/wiki/{{{term}}}",
			},
			[]transformation{wikipediaCanonical},
		},
	}

	return b
}
