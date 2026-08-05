package pullrequest

type Branch struct {
	Name                 string   `json:"name"`
	MergeStrategies      []string `json:"merge_strategies,omitempty"`
	DefaultMergeStrategy string   `json:"default_merge_strategy,omitempty"`
}
