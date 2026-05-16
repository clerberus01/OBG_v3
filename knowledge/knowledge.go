package knowledge

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"omni-bot-go/database"
)

const (
	TTL          = 7 * 24 * time.Hour
	PermanentTTL = 3650 * 24 * time.Hour
)

type RuleMap struct {
	Topic     string            `json:"topic"`
	Source    string            `json:"source,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	Keywords  []string          `json:"keywords,omitempty"`
	Rules     map[string]string `json:"rules"`
	PackRules []Rule            `json:"pack_rules,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type SearchResult struct {
	Topic       string      `json:"topic"`
	Source      string      `json:"source,omitempty"`
	Summary     string      `json:"summary,omitempty"`
	Keywords    []string    `json:"keywords,omitempty"`
	Score       int         `json:"score"`
	RuleMatches []RuleMatch `json:"rule_matches,omitempty"`
}

type RuleMatch struct {
	Pattern  string `json:"pattern,omitempty"`
	Action   string `json:"action,omitempty"`
	Template string `json:"template,omitempty"`
}

type SearchOptions struct {
	Query   string
	Domain  string
	Rule    string
	Pattern string
	Limit   int
}

type Summary struct {
	Topic     string    `json:"topic"`
	Source    string    `json:"source,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	Keywords  []string  `json:"keywords,omitempty"`
	RuleCount int       `json:"rule_count"`
	CreatedAt time.Time `json:"created_at"`
}

type Library struct {
	store *database.Store
}

func New(store *database.Store) *Library {
	return &Library{store: store}
}

func (l *Library) Save(topic string, rules map[string]string) error {
	return l.SaveRuleMap(RuleMap{Topic: topic, Rules: rules})
}

func (l *Library) SaveRuleMap(ruleMap RuleMap) error {
	return l.saveRuleMap(ruleMap, TTL)
}

func (l *Library) SavePermanentRuleMap(ruleMap RuleMap) error {
	return l.saveRuleMap(ruleMap, PermanentTTL)
}

func (l *Library) saveRuleMap(ruleMap RuleMap, ttl time.Duration) error {
	ruleMap.Topic = strings.TrimSpace(ruleMap.Topic)
	if ruleMap.Topic == "" {
		return errors.New("topico de conhecimento vazio")
	}
	if ruleMap.Rules == nil {
		ruleMap.Rules = map[string]string{}
	}
	if ruleMap.CreatedAt.IsZero() {
		ruleMap.CreatedAt = time.Now()
	}
	if len(ruleMap.Keywords) == 0 {
		ruleMap.Keywords = extractKeywords(ruleMap.Topic + " " + ruleMap.Summary + " " + strings.Join(ruleValues(ruleMap), " "))
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(ruleMap); err != nil {
		return err
	}
	return l.store.UpsertKnowledge(ruleMap.Topic, buf.Bytes(), ttl)
}

func (l *Library) SaveKnowledgePack(pack *KnowledgePack, source string) error {
	if pack == nil {
		return errors.New("knowledge pack nil")
	}
	ruleMap := KnowledgePackToRuleMap(pack, source)
	return l.SaveRuleMap(ruleMap)
}

func (l *Library) SavePermanentKnowledgePack(pack *KnowledgePack, source string) error {
	if pack == nil {
		return errors.New("knowledge pack nil")
	}
	ruleMap := KnowledgePackToRuleMap(pack, source)
	return l.SavePermanentRuleMap(ruleMap)
}

func (l *Library) Load(topic string) (RuleMap, error) {
	item, err := l.store.GetKnowledge(topic)
	if err != nil {
		return RuleMap{}, err
	}
	return decodeRuleMap(item.Rules)
}

func (l *Library) IngestText(topic, source, text string) (RuleMap, error) {
	topic = strings.TrimSpace(topic)
	text = strings.TrimSpace(text)
	if topic == "" {
		return RuleMap{}, errors.New("topico de conhecimento vazio")
	}
	if text == "" {
		return RuleMap{}, errors.New("texto de conhecimento vazio")
	}
	ruleMap := RuleMap{
		Topic:     topic,
		Source:    source,
		Summary:   summarize(text, 360),
		Keywords:  extractKeywords(topic + " " + text),
		Rules:     extractRules(text),
		CreatedAt: time.Now(),
	}
	if len(ruleMap.Rules) == 0 {
		ruleMap.Rules["nota"] = summarize(text, 900)
	}
	if err := l.SaveRuleMap(ruleMap); err != nil {
		return RuleMap{}, err
	}
	return ruleMap, nil
}

func (l *Library) IngestFile(topic, path string) (RuleMap, error) {
	clean := filepath.Clean(path)
	raw, err := os.ReadFile(clean)
	if err != nil {
		return RuleMap{}, err
	}
	if topic == "" {
		topic = strings.TrimSuffix(filepath.Base(clean), filepath.Ext(clean))
	}
	return l.IngestText(topic, clean, string(raw))
}

func (l *Library) Search(query string, limit int) ([]SearchResult, error) {
	return l.SearchWithOptions(SearchOptions{Query: query, Limit: limit})
}

func (l *Library) SearchWithOptions(options SearchOptions) ([]SearchResult, error) {
	options.Query = strings.TrimSpace(options.Query)
	options.Domain = strings.TrimSpace(options.Domain)
	options.Rule = strings.TrimSpace(options.Rule)
	options.Pattern = strings.TrimSpace(options.Pattern)
	queryTerms := extractKeywords(options.Query)
	if len(queryTerms) == 0 && options.Domain == "" && options.Rule == "" && options.Pattern == "" {
		return nil, errors.New("consulta vazia")
	}
	items, err := l.store.ListKnowledge(500)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, item := range items {
		ruleMap, err := decodeRuleMap(item.Rules)
		if err != nil {
			continue
		}
		score := scoreRuleMap(ruleMap, queryTerms)
		matches, filterScore := matchRuleMap(ruleMap, options)
		score += filterScore
		if score == 0 {
			continue
		}
		results = append(results, SearchResult{
			Topic:       ruleMap.Topic,
			Source:      ruleMap.Source,
			Summary:     ruleMap.Summary,
			Keywords:    ruleMap.Keywords,
			Score:       score,
			RuleMatches: matches,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Topic < results[j].Topic
		}
		return results[i].Score > results[j].Score
	})
	limit := options.Limit
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	return append([]SearchResult(nil), results[:limit]...), nil
}

func (l *Library) List(limit int) ([]RuleMap, error) {
	items, err := l.store.ListKnowledge(limit)
	if err != nil {
		return nil, err
	}
	out := make([]RuleMap, 0, len(items))
	for _, item := range items {
		ruleMap, err := decodeRuleMap(item.Rules)
		if err != nil {
			continue
		}
		out = append(out, ruleMap)
	}
	return out, nil
}

func (l *Library) Summaries(limit int) ([]Summary, error) {
	items, err := l.List(limit)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(items))
	for _, item := range items {
		out = append(out, Summary{
			Topic:     item.Topic,
			Source:    item.Source,
			Summary:   item.Summary,
			Keywords:  item.Keywords,
			RuleCount: ruleMapRuleCount(item),
			CreatedAt: item.CreatedAt,
		})
	}
	return out, nil
}

func (l *Library) PurgeExpired() (int64, error) {
	return l.store.PurgeExpiredKnowledge()
}

func decodeRuleMap(raw []byte) (RuleMap, error) {
	var out RuleMap
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&out); err != nil {
		return RuleMap{}, err
	}
	return out, nil
}

func extractRules(text string) map[string]string {
	rules := map[string]string{}
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. \t"))
		if len(clean) < 8 {
			continue
		}
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "deve ") || strings.Contains(lower, "must ") || strings.Contains(lower, "nao ") || strings.Contains(lower, "n\u00e3o ") || strings.Contains(lower, "sempre ") || strings.Contains(lower, "evitar ") {
			key := fmt.Sprintf("regra_%02d", len(rules)+1)
			rules[key] = clean
		}
	}
	return rules
}

func extractKeywords(text string) []string {
	counts := map[string]int{}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(word)) < 4 || stopWords[word] {
			continue
		}
		counts[word]++
	}
	words := make([]string, 0, len(counts))
	for word := range counts {
		words = append(words, word)
	}
	sort.Slice(words, func(i, j int) bool {
		if counts[words[i]] == counts[words[j]] {
			return words[i] < words[j]
		}
		return counts[words[i]] > counts[words[j]]
	})
	if len(words) > 24 {
		words = words[:24]
	}
	return words
}

func scoreRuleMap(ruleMap RuleMap, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	haystack := strings.ToLower(ruleMap.Topic + " " + ruleMap.Summary + " " + strings.Join(ruleMap.Keywords, " ") + " " + strings.Join(ruleValues(ruleMap), " "))
	score := 0
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			score++
		}
	}
	return score
}

func matchRuleMap(ruleMap RuleMap, options SearchOptions) ([]RuleMatch, int) {
	domain := normalizeSearchText(options.Domain)
	rule := normalizeSearchText(options.Rule)
	pattern := normalizeSearchText(options.Pattern)
	score := 0
	if domain != "" {
		if !strings.Contains(normalizeSearchText(ruleMap.Topic), domain) {
			return nil, 0
		}
		score += 5
	}
	allRules := searchableRules(ruleMap)
	var matches []RuleMatch
	for _, item := range allRules {
		if rule != "" && !strings.Contains(normalizeSearchText(item.Action+" "+item.Template), rule) {
			continue
		}
		if pattern != "" && !strings.Contains(normalizeSearchText(item.Pattern), pattern) {
			continue
		}
		if rule != "" {
			score += 3
		}
		if pattern != "" {
			score += 3
		}
		if rule != "" || pattern != "" {
			matches = append(matches, item)
		}
	}
	if (rule != "" || pattern != "") && len(matches) == 0 {
		return nil, 0
	}
	return matches, score
}

func searchableRules(ruleMap RuleMap) []RuleMatch {
	if len(ruleMap.PackRules) > 0 {
		out := make([]RuleMatch, 0, len(ruleMap.PackRules))
		for _, rule := range ruleMap.PackRules {
			out = append(out, RuleMatch{
				Pattern:  rule.Pattern,
				Action:   rule.Action,
				Template: rule.Template,
			})
		}
		return out
	}
	keys := make([]string, 0, len(ruleMap.Rules))
	for key := range ruleMap.Rules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]RuleMatch, 0, len(keys))
	for _, key := range keys {
		out = append(out, RuleMatch{
			Action:   key,
			Template: ruleMap.Rules[key],
		})
	}
	return out
}

func normalizeSearchText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func ruleValues(ruleMap RuleMap) []string {
	values := make([]string, 0, len(ruleMap.Rules)+len(ruleMap.PackRules))
	for _, value := range ruleMap.Rules {
		values = append(values, value)
	}
	for _, rule := range ruleMap.PackRules {
		values = append(values, rule.Pattern, rule.Action, rule.Template)
	}
	return values
}

func ruleMapRuleCount(ruleMap RuleMap) int {
	if len(ruleMap.PackRules) > 0 {
		return len(ruleMap.PackRules)
	}
	return len(ruleMap.Rules)
}

func summarize(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

var stopWords = map[string]bool{
	"para": true, "como": true, "deve": true, "devem": true, "esse": true, "essa": true,
	"com": true, "uma": true, "mais": true, "pela": true, "pelo": true, "onde": true,
	"from": true, "that": true, "this": true, "with": true, "must": true, "should": true,
}
