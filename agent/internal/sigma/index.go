package sigma

type RuleIndex struct {
	byCategory map[string][]*CompiledRule
}

func NewRuleIndex() *RuleIndex {
	return &RuleIndex{
		byCategory: make(map[string][]*CompiledRule),
	}
}

func (idx *RuleIndex) Rebuild(rules map[string]*Rule) {
	idx.byCategory = make(map[string][]*CompiledRule)
	for _, rule := range rules {
		compiled := CompileRule(rule)
		category := rule.Logsource.Category
		idx.byCategory[category] = append(idx.byCategory[category], compiled)
	}
}

func (idx *RuleIndex) Match(event map[string]interface{}) []*CompiledRule {
	category, _ := event["category"].(string)
	if category == "" {
		category = "process_creation"
	}
	rules := idx.byCategory[category]

	var matched []*CompiledRule
	for _, rule := range rules {
		if rule.Match(event) {
			matched = append(matched, rule)
		}
	}
	return matched
}
