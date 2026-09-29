package importers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/rutext"
)

// FieldKind is how a catalog field's value is typed.
type FieldKind string

const (
	FieldID      FieldKind = "id"
	FieldText    FieldKind = "text"
	FieldNumber  FieldKind = "number"
	FieldChoice  FieldKind = "choice"
	FieldChoices FieldKind = "choices"
	FieldURL     FieldKind = "url"
	FieldDate    FieldKind = "date"
)

// Field groups: what the robot is, where it comes from and what it costs, its specs, and file-only columns.
const (
	GroupAbout = "about"
	GroupOffer = "offer"
	GroupSpecs = "specs"
	GroupFile  = "file"
)

type store int

const (
	storeNone store = iota
	storeColumn
	storeSpec
	storeRaw
)

// Choice is one allowed value of a choice field.
type Choice struct {
	Code    string   `json:"code"`
	Label   string   `json:"label"`
	aliases []string `json:"-"`
}

// Field is one catalog field as the admin edits it and as catalog files carry it.
type Field struct {
	Code     string    `json:"code"`
	Label    string    `json:"label"`
	Unit     string    `json:"unit,omitempty"`
	Kind     FieldKind `json:"kind"`
	Group    string    `json:"group"`
	Choices  []Choice  `json:"choices,omitempty"`
	Min      *float64  `json:"min,omitempty"`
	Max      *float64  `json:"max,omitempty"`
	Integer  bool      `json:"integer,omitempty"`
	Percent  bool      `json:"percent,omitempty"`
	MaxLen   int       `json:"max_len,omitempty"`
	Required bool      `json:"required,omitempty"`
	Editable bool      `json:"editable"`

	// headers are other spellings of the column header, compared after headerKey.
	headers []string
	// exact headers match only as written, case included: the organizer file has both «тип» and «Тип».
	exact  []string
	store  store
	rawKey string
}

func num(v float64) *float64 { return &v }

var fields = []Field{
	{Code: "id", Label: "Идентификатор", Kind: FieldID, Group: GroupFile, headers: []string{"id", "uuid"}},
	{Code: "name", Label: "Название", Kind: FieldText, Group: GroupAbout, MaxLen: 300, Required: true, Editable: true,
		headers: []string{"наименование", "модель", "name"}, store: storeColumn},
	{Code: "vendor", Label: "Компания", Kind: FieldText, Group: GroupAbout, MaxLen: 300, Editable: true,
		headers: []string{"вендор", "производитель", "поставщик", "vendor"}, store: storeColumn},
	{Code: "family", Label: "Тип", Kind: FieldChoice, Group: GroupAbout, Editable: true, Choices: familyChoices(),
		exact: []string{"Тип"}, headers: []string{"тип робота", "группа", "family"}, store: storeColumn},
	{Code: "kind", Label: "Вид", Kind: FieldChoice, Group: GroupAbout, Editable: true, Choices: []Choice{
		{Code: "brs", Label: "Робот", aliases: []string{"брс", "brs"}},
		{Code: "bas", Label: "Беспилотник", aliases: []string{"бас", "дрон", "bas"}},
		{Code: "software", Label: "ПО", aliases: []string{"программное обеспечение", "software"}},
	}, exact: []string{"тип"}, headers: []string{"вид решения", "kind"}, store: storeColumn},
	{Code: "subtype", Label: "Подтип", Kind: FieldText, Group: GroupAbout, MaxLen: 300, Editable: true,
		headers: []string{"subtype"}, store: storeColumn},
	{Code: "status", Label: "Статус", Kind: FieldChoice, Group: GroupAbout, Editable: true, Choices: []Choice{
		{Code: "operation", Label: "В эксплуатации", aliases: []string{"эксплуатация", "operation"}},
		{Code: "piloting", Label: "Пилот", aliases: []string{"пилотный проект", "пилотирование", "piloting"}},
		{Code: "rnd", Label: "НИОКР", aliases: []string{"разработка", "rnd"}},
	}, headers: []string{"стадия", "status"}, store: storeColumn},
	{Code: "industry", Label: "Отрасль", Kind: FieldText, Group: GroupAbout, MaxLen: 300, Editable: true,
		headers: []string{"industry"}, store: storeColumn},
	{Code: "scenario", Label: "Сценарий", Kind: FieldText, Group: GroupAbout, MaxLen: 1000, Editable: true,
		headers: []string{"сценарий применения", "scenario"}, store: storeColumn},
	{Code: "cases", Label: "Кейсы", Kind: FieldText, Group: GroupFile, MaxLen: 4000,
		headers: []string{"кейс", "cases"}},
	{Code: "description", Label: "Описание", Kind: FieldText, Group: GroupAbout, MaxLen: 4000, Editable: true,
		headers: []string{"description"}, store: storeRaw, rawKey: "описание"},
	{Code: "region", Label: "Регион", Kind: FieldText, Group: GroupAbout, MaxLen: 300, Editable: true,
		headers: []string{"region"}, store: storeRaw, rawKey: "Регион"},
	{Code: "ugt", Label: "УГТ", Kind: FieldNumber, Group: GroupAbout, Min: num(1), Max: num(9), Integer: true,
		Editable: true, headers: []string{"уровень готовности технологии", "ugt"}, store: storeRaw, rawKey: "УГТ"},
	{Code: "market", Label: "Рыночный потенциал", Kind: FieldNumber, Group: GroupAbout, Min: num(0), Max: num(10),
		Editable: true, headers: []string{"рын потенциал", "market"}, store: storeRaw, rawKey: "Рын Потенциал"},
	{Code: "price_rub", Label: "Цена", Unit: "₽", Kind: FieldNumber, Group: GroupOffer, Min: num(0), Max: num(1e11),
		Editable: true, headers: []string{"цена изделия", "стоимость", "price_rub"}, store: storeColumn},
	{Code: "object_types", Label: "Объекты", Kind: FieldChoices, Group: GroupOffer, Editable: true, Choices: []Choice{
		{Code: "warehouse", Label: "Склад", aliases: []string{"склады", "логистический центр", "warehouse"}},
		{Code: "airport", Label: "Аэропорт", aliases: []string{"аэропорты", "airport"}},
		{Code: "hospital", Label: "Медучреждение", aliases: []string{"больница", "больницы", "клиника", "медицинское учреждение", "hospital"}},
	}, headers: []string{"типы объектов", "object_types"}, store: storeRaw, rawKey: "object_types"},
	{Code: "source_url", Label: "Источник", Kind: FieldURL, Group: GroupOffer, MaxLen: 2000, Editable: true,
		headers: []string{"ссылка", "сайт", "source_url"}, store: storeColumn},
	{Code: "confidence", Label: "Источник ТТХ", Kind: FieldChoice, Group: GroupOffer, Editable: true, Choices: []Choice{
		{Code: "measured", Label: "Измерения", aliases: []string{"измерено", "measured"}},
		{Code: "vendor", Label: "Данные производителя", aliases: []string{"производитель", "vendor"}},
		{Code: "assumed", Label: "Оценка", aliases: []string{"допущение", "assumed"}},
	}, headers: []string{"достоверность", "confidence"}, store: storeSpec},
	{Code: "sourced_at", Label: "Дата источника", Kind: FieldDate, Group: GroupOffer, Editable: true,
		headers: []string{"sourced_at"}, store: storeSpec},
	{Code: "payload_kg", Label: "Грузоподъёмность", Unit: "кг", Kind: FieldNumber, Group: GroupSpecs, Min: num(0),
		Max: num(100000), Editable: true, headers: []string{"полезная нагрузка", "нагрузка", "payload_kg"}, store: storeSpec},
	{Code: "mass_kg", Label: "Масса", Unit: "кг", Kind: FieldNumber, Group: GroupSpecs, Min: num(0), Max: num(100000),
		Editable: true, headers: []string{"вес", "mass_kg"}, store: storeSpec},
	{Code: "length_mm", Label: "Длина", Unit: "мм", Kind: FieldNumber, Group: GroupSpecs, Min: num(0), Max: num(100000),
		Editable: true, headers: []string{"length_mm"}, store: storeSpec},
	{Code: "width_mm", Label: "Ширина", Unit: "мм", Kind: FieldNumber, Group: GroupSpecs, Min: num(0), Max: num(100000),
		Editable: true, headers: []string{"width_mm"}, store: storeSpec},
	{Code: "height_mm", Label: "Высота", Unit: "мм", Kind: FieldNumber, Group: GroupSpecs, Min: num(0), Max: num(100000),
		Editable: true, headers: []string{"height_mm"}, store: storeSpec},
	{Code: "speed_mps", Label: "Скорость", Unit: "м/с", Kind: FieldNumber, Group: GroupSpecs, Min: num(0), Max: num(150),
		Editable: true, headers: []string{"максимальная скорость", "speed_mps"}, store: storeSpec},
	{Code: "endurance_h", Label: "Время работы", Unit: "ч", Kind: FieldNumber, Group: GroupSpecs, Min: num(0),
		Max: num(10000), Editable: true, headers: []string{"автономность", "время полёта", "endurance_h"}, store: storeSpec},
	{Code: "charge_min", Label: "Время зарядки", Unit: "мин", Kind: FieldNumber, Group: GroupSpecs, Min: num(0),
		Max: num(10000), Editable: true, headers: []string{"время заряда", "зарядка", "charge_min"}, store: storeSpec},
	{Code: "nav_type", Label: "Навигация", Kind: FieldText, Group: GroupSpecs, MaxLen: 200, Editable: true,
		headers: []string{"тип навигации", "nav_type"}, store: storeSpec},
	{Code: "pos_accuracy_mm", Label: "Точность позиционирования", Unit: "мм", Kind: FieldNumber, Group: GroupSpecs,
		Min: num(0), Max: num(100000), Editable: true, headers: []string{"точность", "pos_accuracy_mm"}, store: storeSpec},
	{Code: "min_aisle_mm", Label: "Мин. проход", Unit: "мм", Kind: FieldNumber, Group: GroupSpecs, Min: num(0),
		Max: num(100000), Editable: true,
		headers: []string{"минимальный проход", "минимальная ширина прохода", "ширина прохода", "min_aisle_mm"}, store: storeSpec},
	{Code: "turn_radius_mm", Label: "Радиус разворота", Unit: "мм", Kind: FieldNumber, Group: GroupSpecs, Min: num(0),
		Max: num(100000), Editable: true, headers: []string{"радиус разворота", "turn_radius_mm"}, store: storeSpec},
	{Code: "temp_min_c", Label: "Температура от", Unit: "°C", Kind: FieldNumber, Group: GroupSpecs, Min: num(-100),
		Max: num(200), Editable: true, headers: []string{"минимальная температура", "temp_min_c"}, store: storeSpec},
	{Code: "temp_max_c", Label: "Температура до", Unit: "°C", Kind: FieldNumber, Group: GroupSpecs, Min: num(-100),
		Max: num(200), Editable: true, headers: []string{"максимальная температура", "temp_max_c"}, store: storeSpec},
	{Code: "lifetime_years", Label: "Срок службы", Unit: "лет", Kind: FieldNumber, Group: GroupSpecs, Min: num(0),
		Max: num(100), Editable: true, headers: []string{"lifetime_years"}, store: storeSpec},
	{Code: "service_pct_year", Label: "Сервис в год", Unit: "%", Kind: FieldNumber, Group: GroupSpecs, Min: num(0),
		Max: num(1), Percent: true, Editable: true, headers: []string{"обслуживание в год", "service_pct_year"}, store: storeSpec},
	{Code: "photo", Label: "Фото", Kind: FieldURL, Group: GroupFile, MaxLen: 2000,
		headers: []string{"фотография", "изображение", "картинка", "photo"}},
	{Code: StampField, Label: "Выгрузка", Kind: FieldID, Group: GroupFile},
}

// StampField is the column a catalog export writes its id into, so an upload of that file merges three ways.
const StampField = "stamp"

func familyChoices() []Choice {
	aliases := map[string][]string{
		catalog.FamilyMobile:      {"мобильный робот"},
		catalog.FamilyUAV:         {"беспилотник", "беспилотники", "дрон", "дроны", "бпла"},
		catalog.FamilyGround:      {"наземные тс", "наземный транспорт"},
		catalog.FamilyMarine:      {"морской робот"},
		catalog.FamilyStationary:  {"стационарная система", "стационарные системы"},
		catalog.FamilyManipulator: {"мобильные манипуляторы", "манипулятор", "манипуляторы"},
		catalog.FamilyHumanoid:    {"антропоморфный робот", "гуманоид", "гуманоиды"},
		catalog.FamilySoftware:    {"по брс", "по", "программное обеспечение"},
		catalog.FamilyOther:       {},
	}
	out := make([]Choice, 0, len(aliases))
	for _, code := range catalog.FamilyCodes() {
		out = append(out, Choice{Code: code, Label: catalog.FamilyLabel(code), aliases: append(aliases[code], code)})
	}
	return out
}

// Fields lists the catalog fields in file column order.
func Fields() []Field {
	return slices.Clone(fields)
}

// FieldByCode finds a field by its code.
func FieldByCode(code string) (Field, bool) {
	for _, f := range fields {
		if f.Code == code {
			return f, true
		}
	}
	return Field{}, false
}

// FieldForHeader finds the field a file column header names, or false.
func FieldForHeader(header string) (Field, bool) {
	trimmed := strings.TrimSpace(header)
	for _, f := range fields {
		if slices.Contains(f.exact, trimmed) {
			return f, true
		}
	}
	key := headerKey(trimmed)
	if key == "" {
		return Field{}, false
	}
	for _, f := range fields {
		if !slices.Contains(f.exact, f.Label) && (headerKey(f.Label) == key || headerKey(f.Header()) == key) {
			return f, true
		}
		for _, h := range f.headers {
			if headerKey(h) == key {
				return f, true
			}
		}
	}
	// A header may carry its unit: «Вес, кг», «Вес (кг)».
	if i := strings.LastIndexAny(trimmed, ",("); i > 0 {
		return FieldForHeader(trimmed[:i])
	}
	return Field{}, false
}

// Header is the column header files carry: the label and its unit, «Грузоподъёмность, кг».
func (f Field) Header() string {
	if f.Unit == "" {
		return f.Label
	}
	return f.Label + ", " + f.Unit
}

// headerKey folds a header or a label for matching: case, ё, punctuation and spacing do not count.
func headerKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("ё", "е", "²", "2", "³", "3", "×", "x").Replace(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '%' {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
			continue
		}
		space = true
	}
	return b.String()
}

// FieldError is a value a field does not accept, with the reason in Russian.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }

func (f Field) fail(format string, args ...any) *FieldError {
	return &FieldError{Field: f.Code, Message: f.Label + ": " + fmt.Sprintf(format, args...)}
}

// Parse reads a file cell. An empty cell gives nil. Text keeps its no-break spaces as delivered.
func (f Field) Parse(cell string) (any, error) {
	s := strings.TrimSpace(cell)
	if s == "" {
		return nil, nil
	}
	switch f.Kind {
	case FieldID:
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, f.fail("нужен идентификатор из выгрузки каталога, а в ячейке «%s».", clip(s))
		}
		return id.String(), nil
	case FieldText:
		return f.checkText(Text(s))
	case FieldURL:
		return f.checkURL(s)
	case FieldNumber:
		v, err := f.parseNumber(s)
		if err != nil {
			return nil, err
		}
		return f.checkNumber(v)
	case FieldChoice:
		code, ok := f.choice(s)
		if !ok {
			return nil, f.fail("нет такого значения: «%s». Подходят: %s.", clip(s), f.choiceLabels())
		}
		return code, nil
	case FieldChoices:
		var codes []string
		for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '/' }) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			code, ok := f.choice(part)
			if !ok {
				return nil, f.fail("нет такого значения: «%s». Подходят: %s.", clip(part), f.choiceLabels())
			}
			codes = append(codes, code)
		}
		return f.orderChoices(codes), nil
	case FieldDate:
		return f.parseDate(s)
	}
	return nil, f.fail("поле не читается из файла.")
}

// FromJSON reads a value sent by the admin page, in stored units. JSON null clears the field.
func (f Field) FromJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	switch f.Kind {
	case FieldNumber:
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, f.fail("нужно число.")
		}
		return f.checkNumber(v)
	case FieldChoices:
		var codes []string
		if err := json.Unmarshal(raw, &codes); err != nil {
			return nil, f.fail("нужен список значений.")
		}
		for _, c := range codes {
			if !f.hasChoice(c) {
				return nil, f.fail("нет такого значения. Выберите значение из списка.")
			}
		}
		return f.orderChoices(codes), nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, f.fail("нужен текст.")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	switch f.Kind {
	case FieldText:
		return f.checkText(Text(s))
	case FieldURL:
		return f.checkURL(s)
	case FieldChoice:
		if !f.hasChoice(s) {
			return nil, f.fail("нет такого значения. Выберите значение из списка.")
		}
		return s, nil
	case FieldDate:
		return f.parseDate(s)
	}
	return nil, f.fail("поле нельзя изменить.")
}

// Format writes a value into a file cell: codes as Russian labels, numbers with a decimal comma, percent for shares.
func (f Field) Format(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case float64:
		if f.Percent {
			x = x * 100
		}
		return strings.Replace(strconv.FormatFloat(roundDigits(x, 6), 'f', -1, 64), ".", ",", 1)
	case []string:
		labels := make([]string, 0, len(x))
		for _, c := range x {
			labels = append(labels, f.ChoiceLabel(c))
		}
		return strings.Join(labels, ", ")
	case string:
		switch f.Kind {
		case FieldChoice:
			return f.ChoiceLabel(x)
		case FieldDate:
			if t, err := time.Parse(time.DateOnly, x); err == nil {
				return t.Format("02.01.2006")
			}
		}
		return x
	}
	return fmt.Sprint(v)
}

// ChoiceLabel is the Russian label of a choice code, or the code when the field has no such choice.
func (f Field) ChoiceLabel(code string) string {
	for _, c := range f.Choices {
		if c.Code == code {
			return c.Label
		}
	}
	return code
}

func (f Field) hasChoice(code string) bool {
	for _, c := range f.Choices {
		if c.Code == code {
			return true
		}
	}
	return false
}

func (f Field) choice(s string) (string, bool) {
	key := headerKey(s)
	for _, c := range f.Choices {
		if c.Code == s || headerKey(c.Label) == key {
			return c.Code, true
		}
		for _, a := range c.aliases {
			if headerKey(a) == key {
				return c.Code, true
			}
		}
	}
	return "", false
}

func (f Field) choiceLabels() string {
	labels := make([]string, 0, len(f.Choices))
	for _, c := range f.Choices {
		labels = append(labels, "«"+c.Label+"»")
	}
	return strings.Join(labels, ", ")
}

// orderChoices drops repeats and puts codes in the field's order, so equal sets compare equal.
func (f Field) orderChoices(codes []string) any {
	var out []string
	for _, c := range f.Choices {
		if slices.Contains(codes, c.Code) {
			out = append(out, c.Code)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (f Field) checkText(s string) (any, error) {
	if n := utf8.RuneCountInString(s); f.MaxLen > 0 && n > f.MaxLen {
		return nil, f.fail("не длиннее %s знаков, а сейчас %s.", rutext.Num(float64(f.MaxLen), 0), rutext.Num(float64(n), 0))
	}
	return s, nil
}

func (f Field) checkURL(s string) (any, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, f.fail("нужна ссылка, которая начинается с http:// или https://.")
	}
	if f.MaxLen > 0 && len(s) > f.MaxLen {
		return nil, f.fail("ссылка длиннее %s знаков.", rutext.Num(float64(f.MaxLen), 0))
	}
	return s, nil
}

func (f Field) checkNumber(v float64) (any, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, f.fail("нужно число.")
	}
	if f.Integer && v != math.Trunc(v) {
		return nil, f.fail("нужно целое число.")
	}
	if f.Min == nil || f.Max == nil || (v >= *f.Min && v <= *f.Max) {
		return v, nil
	}
	lo, hi := *f.Min, *f.Max
	if f.Percent {
		lo, hi = lo*100, hi*100
	}
	unit := f.Unit
	if unit != "" && unit != "%" && unit != "°C" {
		unit = " " + unit
	}
	return nil, f.fail("число от %s до %s%s.", rutext.Num(lo, 2), rutext.Num(hi, 2), unit)
}

// parseNumber reads 1 500, 2,5, 2.5, 2 700 000,00 ₽ and 25 %: digit groups, either decimal mark, the field's unit.
func (f Field) parseNumber(s string) (float64, error) {
	t := strings.TrimSpace(s)
	for _, suffix := range []string{f.Unit, "руб.", "руб", "р.", "%"} {
		if suffix != "" && strings.HasSuffix(strings.ToLower(t), strings.ToLower(suffix)) {
			t = strings.TrimSpace(t[:len(t)-len(suffix)])
		}
	}
	t = strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "", "\u2009", "", "\u2212", "-").Replace(t)
	if strings.Count(t, ",") == 1 && !strings.Contains(t, ".") {
		t = strings.Replace(t, ",", ".", 1)
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, f.fail("нужно число, а в ячейке «%s».", clip(s))
	}
	if f.Percent {
		v = v / 100
	}
	return v, nil
}

func (f Field) parseDate(s string) (any, error) {
	for _, layout := range []string{time.DateOnly, "02.01.2006", "2.1.2006", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Year() < 1990 || t.Year() > 2100 {
				break
			}
			return t.Format(time.DateOnly), nil
		}
	}
	return nil, f.fail("нужна дата в виде 28.09.2026, а в ячейке «%s».", clip(s))
}

func roundDigits(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}

// clip shortens a cell quoted in an error message.
func clip(s string) string {
	const max = 40
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "..."
}
