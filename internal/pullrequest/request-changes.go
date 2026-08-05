package pullrequest

var requestChangesSpec = actionSpec{
	name:     "request-changes",
	aliases:  []string{"requestChanges", "requestchanges"},
	short:    "Request changes on a pullrequest by its <pullrequest-id>. If not provided, it will try to request changes on the only open pullrequest.",
	whatIf:   "Requesting changes on",
	errVerb:  "request changes on",
	endpoint: "request-changes",
	post:     true,
}

func init() {
	Command.AddCommand(newActionCommand(requestChangesSpec))
}
