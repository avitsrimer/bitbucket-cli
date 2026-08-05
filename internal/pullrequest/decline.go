package pullrequest

var declineSpec = actionSpec{
	name:     "decline",
	short:    "decline a pullrequest by its <pullrequest-id>. If not provided, it will try to decline the only open pullrequest.",
	whatIf:   "Declining",
	errVerb:  "decline",
	endpoint: "decline",
	post:     true,
}

func init() {
	Command.AddCommand(newActionCommand(declineSpec))
}
