package matching

const Version = "match-v4"

const (
	KindHard    = "hard"
	KindSoft    = "soft"
	KindMissing = "missing_evidence"
)

const (
	OutcomePass    = "pass"
	OutcomeFail    = "fail"
	OutcomeUnknown = "unknown"
)

type Step struct {
	RuleID         string   `json:"rule_id"`
	Kind           string   `json:"kind"`
	Outcome        string   `json:"outcome"`
	TaskCode       string   `json:"task_code,omitempty"`
	CapabilityCode string   `json:"capability_code,omitempty"`
	ObjectField    string   `json:"object_field,omitempty"`
	ObjectValue    *float64 `json:"object_value"`
	ObjectUnit     string   `json:"object_unit,omitempty"`
	SolutionField  string   `json:"solution_field,omitempty"`
	SolutionValue  *float64 `json:"solution_value"`
	SolutionUnit   string   `json:"solution_unit,omitempty"`
	SourceURL      *string  `json:"source_url,omitempty"`
	Confidence     *string  `json:"confidence,omitempty"`
	Text           string   `json:"text"`
}

func splitSteps(steps []Step) (hard, soft, missing []Step) {
	hard = []Step{}
	soft = []Step{}
	missing = []Step{}
	for _, st := range steps {
		switch st.Kind {
		case KindHard:
			hard = append(hard, st)
		case KindSoft:
			soft = append(soft, st)
		default:
			missing = append(missing, st)
		}
	}
	return hard, soft, missing
}

func reasonsFromSteps(status string, steps []Step) []string {
	if status == StatusRecommended {
		return []string{"Подходит: процесс совпадает, ограничения объекта не нарушены."}
	}
	out := make([]string, 0, len(steps))
	for _, st := range steps {
		if st.Outcome == OutcomePass {
			continue
		}
		if st.Text != "" {
			out = append(out, st.Text)
		}
	}
	return dedupe(out)
}

// withFieldSource points a step at the source of the robot value it checked, when the catalog names one.
func withFieldSource(c Candidate, st Step) Step {
	src, ok := c.FieldSources[st.SolutionField]
	if !ok || st.SolutionField == "" {
		return st
	}
	if src.SourceURL != "" {
		u := src.SourceURL
		st.SourceURL = &u
	}
	if src.Confidence != "" {
		conf := src.Confidence
		st.Confidence = &conf
	}
	return st
}
