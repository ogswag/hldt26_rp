package simbuild

import (
	"encoding/json"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/sim"
)

// SuggestionFrom reads the robot a calculation suggested and how many of them it needs, from the stored result.
func SuggestionFrom(results []byte) (*Suggestion, error) {
	var res econ.Result
	if len(results) == 0 || json.Unmarshal(results, &res) != nil {
		return nil, noRobot()
	}
	return SuggestionOf(res)
}

func noRobot() error {
	return &sim.InputError{Msg: "Симуляции нужен робот. Откройте Расчёт: он предложит робота, который подходит к объекту."}
}

// SuggestionOf reads the same from a result already decoded.
func SuggestionOf(res econ.Result) (*Suggestion, error) {
	if res.Match.Best == "" {
		return nil, noRobot()
	}
	for _, it := range res.Match.Items {
		if it.SolutionID != res.Match.Best {
			continue
		}
		if it.Estimate == nil || it.Estimate.FleetSize < 1 {
			return nil, &sim.InputError{Msg: "У предложенного робота " + it.Name + " нет цены в каталоге, поэтому расчёт не знает, сколько их нужно. Выберите робота и количество на вкладке Роботы."}
		}
		return &Suggestion{SolutionID: it.SolutionID, Quantity: it.Estimate.FleetSize, ProcessCodes: it.Estimate.ProcessCodes}, nil
	}
	return nil, noRobot()
}
