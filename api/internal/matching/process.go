package matching

import (
	"strings"
)

type processKind int

const (
	processMatch processKind = iota
	processUnknown
	processMismatch
)

const processMatchReason = "Процесс совпадает с типом объекта."

var warehouseNeedles = []string{
	"склад", "внутрисклад", "сортировка грузов", "инвентаризац",
	"штабел", "шаттл", "комплектов", "паллет", "pallet",
	"внутрипроизводственн", "погрузчик",
}

var airportNeedles = []string{
	"аэропорт", "багаж", "перрон", "airside", "тягач",
	"закрытых площадках", "наземного обслуживания",
}

var hospitalNeedles = []string{
	"больниц", "медицин", "биоматериал", "внутрибольнич", "поликлиник",
	"склиф", "палат", "медучрежд",
}

var allObjectNeedles = []string{
	"уборка помещений", "робот-уборщик", "робот уборщик", "поломо",
}

var familyBanNeedles = []string{
	"морские роботы", "подводн", "тнпа", "катер", "катамаран", "мелковод",
	"безэкипажн", "наводн", "самолёт", "мультиротор", "vtol", "бпла",
	"октокоптер", "коптер", "вспашка", "сбор урожая", "доильн", "птицевод",
	"агробот", "беспилотный трактор", "сбор плодов", "внесение веществ",
	"таксация", "лесопатолог", "лесосек", "внутритрубн", "трубопровод",
	"теплотрасс", "пассажир", "трамвай", "метро", "такси",
	"общественная перевозка", "бульдозер", "асфальтоукладчик",
	"беспилотный каток", "сварочн", "демонтаж",
}

func classifyProcessStep(objectType string, c Candidate) (processKind, Step) {
	kind, text := classifyProcess(objectType, c)
	st := Step{RuleID: "process_object_type", Text: text}
	switch kind {
	case processMatch:
		st.Kind = KindHard
		st.Outcome = OutcomePass
	case processUnknown:
		st.Kind = KindMissing
		st.Outcome = OutcomeUnknown
	default:
		st.Kind = KindHard
		st.Outcome = OutcomeFail
	}
	return kind, st
}

func classifyProcess(objectType string, c Candidate) (processKind, string) {
	if types := typedObjects(c.ObjectTypes); len(types) > 0 {
		if types[objectType] {
			return processMatch, processMatchReason
		}
		return processMismatch, "В карточке ТТХ этот тип объекта не указан. Решение исключено для выбранного объекта. Выберите другую модель или добавьте к сравнению с предупреждением."
	}
	if banned, why := bannedFamily(c); banned {
		return processMismatch, why
	}
	text := processText(c)
	if containsAny(text, allObjectNeedles) {
		return processMatch, processMatchReason
	}
	own := needlesFor(objectType)
	if containsAny(text, own) {
		return processMatch, processMatchReason
	}
	for _, other := range []string{"warehouse", "airport", "hospital"} {
		if other == objectType {
			continue
		}
		if containsAny(text, needlesFor(other)) {
			return processMismatch, "Сценарий каталога относится к другому типу объекта. Решение исключено. Выберите модель под этот объект или добавьте к сравнению с предупреждением."
		}
	}
	return processUnknown, "В каталоге нет явного процесса для этого типа объекта. Нельзя рекомендовать без проверки. Уточните сценарий или добавьте к сравнению с предупреждением."
}

func typedObjects(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, t := range in {
		switch strings.TrimSpace(t) {
		case "warehouse", "airport", "hospital":
			out[t] = true
		}
	}
	return out
}

func needlesFor(objectType string) []string {
	switch objectType {
	case "warehouse":
		return warehouseNeedles
	case "airport":
		return airportNeedles
	case "hospital":
		return hospitalNeedles
	default:
		return nil
	}
}

func bannedFamily(c Candidate) (bool, string) {
	family := fold(c.Family)
	if family == "морские роботы" || family == "бас" {
		return true, "Класс решения (" + c.Family + ") не подходит для этого объекта. Выберите складской, аэропортовый или больничный наземный робот, либо добавьте к сравнению с предупреждением."
	}
	t := processText(c)
	if containsAny(t, familyBanNeedles) {
		return true, "Класс решения (морская техника, БАС, агро, пассажирский транспорт и подобные) не подходит для этого объекта. Выберите другую модель или добавьте к сравнению с предупреждением."
	}
	return false, ""
}

func processText(c Candidate) string {
	var b strings.Builder
	write := func(s string) {
		if s == "" {
			return
		}
		b.WriteString(s)
		b.WriteByte(' ')
	}
	write(c.Name)
	write(c.Family)
	write(deref(c.Kind))
	write(deref(c.Subtype))
	write(deref(c.Scenario))
	write(c.Description)
	for _, u := range c.Uses {
		write(u.Industry)
		write(u.Scenario)
		write(u.Cases)
	}
	return fold(b.String())
}

func fold(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		if n == "" {
			continue
		}
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}
