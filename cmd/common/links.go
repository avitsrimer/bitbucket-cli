package common

type Links struct {
	Self           *Link  `json:"self,omitempty"            mapstructure:"self"`
	HTML           *Link  `json:"html,omitempty"            mapstructure:"html"`
	Avatar         *Link  `json:"avatar,omitempty"          mapstructure:"avatar"`
	Branches       *Link  `json:"branches,omitempty"        mapstructure:"branches"`
	Forks          *Link  `json:"forks,omitempty"           mapstructure:"forks"`
	Commits        *Link  `json:"commits,omitempty"         mapstructure:"commits"`
	PullRequests   *Link  `json:"pullrequests,omitempty"    mapstructure:"pullrequests"`
	Approve        *Link  `json:"approve,omitempty"         mapstructure:"approve"`
	RequestChanges *Link  `json:"request-changes,omitempty" mapstructure:"request-changes"`
	Diff           *Link  `json:"diff,omitempty"            mapstructure:"diff"`
	DiffStat       *Link  `json:"diffstat,omitempty"        mapstructure:"diffstat"`
	Patch          *Link  `json:"patch,omitempty"           mapstructure:"patch"`
	Comments       *Link  `json:"comments,omitempty"        mapstructure:"comments"`
	Activity       *Link  `json:"activity,omitempty"        mapstructure:"activity"`
	Merge          *Link  `json:"merge,omitempty"           mapstructure:"merge"`
	Decline        *Link  `json:"decline,omitempty"         mapstructure:"decline"`
	Statuses       *Link  `json:"statuses,omitempty"        mapstructure:"statuses"`
	Tags           *Link  `json:"tags,omitempty"            mapstructure:"tags"`
	Watchers       *Link  `json:"watchers,omitempty"        mapstructure:"watchers"`
	Downloads      *Link  `json:"downloads,omitempty"       mapstructure:"downloads"`
	Source         *Link  `json:"source,omitempty"          mapstructure:"source"`
	Clone          []Link `json:"clone,omitempty"           mapstructure:"clone"`
	Hooks          *Link  `json:"hooks,omitempty"           mapstructure:"hooks"`
	Steps          *Link  `json:"steps,omitempty"           mapstructure:"steps"`
}

// IsEmpty tells if there is no link defined
func (links Links) IsEmpty() bool {
	for _, link := range []*Link{
		links.Self, links.HTML, links.Avatar, links.Branches, links.Forks, links.Commits,
		links.PullRequests, links.Approve, links.RequestChanges, links.Diff, links.DiffStat,
		links.Patch, links.Comments, links.Activity, links.Merge, links.Decline, links.Statuses,
		links.Tags, links.Watchers, links.Downloads, links.Source, links.Hooks, links.Steps,
	} {
		if link != nil {
			return false
		}
	}
	return len(links.Clone) == 0
}
