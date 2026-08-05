package common

type Links struct {
	Self           *Link  `json:"self,omitempty"`
	HTML           *Link  `json:"html,omitempty"`
	Avatar         *Link  `json:"avatar,omitempty"`
	Branches       *Link  `json:"branches,omitempty"`
	Forks          *Link  `json:"forks,omitempty"`
	Commits        *Link  `json:"commits,omitempty"`
	PullRequests   *Link  `json:"pullrequests,omitempty"`
	Approve        *Link  `json:"approve,omitempty"`
	RequestChanges *Link  `json:"request-changes,omitempty"`
	Diff           *Link  `json:"diff,omitempty"`
	DiffStat       *Link  `json:"diffstat,omitempty"`
	Patch          *Link  `json:"patch,omitempty"`
	Comments       *Link  `json:"comments,omitempty"`
	Activity       *Link  `json:"activity,omitempty"`
	Merge          *Link  `json:"merge,omitempty"`
	Decline        *Link  `json:"decline,omitempty"`
	Statuses       *Link  `json:"statuses,omitempty"`
	Tags           *Link  `json:"tags,omitempty"`
	Watchers       *Link  `json:"watchers,omitempty"`
	Downloads      *Link  `json:"downloads,omitempty"`
	Source         *Link  `json:"source,omitempty"`
	Clone          []Link `json:"clone,omitempty"`
	Hooks          *Link  `json:"hooks,omitempty"`
	Steps          *Link  `json:"steps,omitempty"`
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
