package objects

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"moscow_hackathon_2026/api/internal/rutext"
	"sort"
	"strconv"
	"strings"
)

const (
	TypeNumber    = "number"
	TypeInteger   = "integer"
	TypeString    = "string"
	TypeBoolean   = "boolean"
	TypeEnum      = "enum"
	TypeMultiEnum = "multi_enum"
	TypeTimeRange = "time_range"
	// TypeDimensions is a box size: Dimensions in the field unit, each part within Min and Max.
	TypeDimensions = "dimensions"
)

const UnknownLabel = "Неизвестно"

var ErrUnknownType = errors.New("objects.unknown_type")

type TypeInfo struct {
	Type  string `json:"type"`
	Label string `json:"label"`
}

type EnumOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Dimensions is a box size in millimetres.
type Dimensions struct {
	Length float64 `json:"length"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Field struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	Short        string       `json:"short,omitempty"` // fits one line of the form; empty when Label does
	Unit         string       `json:"unit"`
	Type         string       `json:"type"`
	Required     bool         `json:"required"`
	AllowUnknown bool         `json:"allow_unknown"`
	Default      any          `json:"default"`
	Min          *float64     `json:"min,omitempty"`
	Max          *float64     `json:"max,omitempty"`
	Note         string       `json:"note,omitempty"`
	Help         *string      `json:"help,omitempty"`
	Options      []EnumOption `json:"options,omitempty"`
	Aliases      []string     `json:"aliases,omitempty"`
}

type Group struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Tab    string  `json:"tab"`
	Fields []Field `json:"fields"`
}

// Tab is one page of the object form. Key tabs must be reviewed before the project is calculated.
type Tab struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Key   bool   `json:"key"`
}

type Schema struct {
	Type   string  `json:"type"`
	Label  string  `json:"label"`
	Tabs   []Tab   `json:"tabs"`
	Groups []Group `json:"groups"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Details []FieldError
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "objects.validate"
	}
	return e.Details[0].Message
}

func Types() []TypeInfo {
	return []TypeInfo{
		{Type: Warehouse, Label: "Склад"},
		{Type: Airport, Label: "Аэропорт"},
		{Type: Hospital, Label: "Медучреждение"},
	}
}

func SchemaFor(objectType string) (Schema, error) {
	switch objectType {
	case Warehouse:
		return warehouseSchema(), nil
	case Airport:
		return airportSchema(), nil
	case Hospital:
		return hospitalSchema(), nil
	default:
		return Schema{}, fmt.Errorf("objects.schema: %w", ErrUnknownType)
	}
}

func Defaults(objectType string) (map[string]any, error) {
	s, err := SchemaFor(objectType)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any)
	for _, g := range s.Groups {
		for _, f := range g.Fields {
			out[f.ID] = f.Default
		}
	}
	return out, nil
}

func Fields(s Schema) []Field {
	n := 0
	for _, g := range s.Groups {
		n += len(g.Fields)
	}
	out := make([]Field, 0, n)
	for _, g := range s.Groups {
		out = append(out, g.Fields...)
	}
	return out
}

func Validate(objectType string, params json.RawMessage) error {
	s, err := SchemaFor(objectType)
	if err != nil {
		return err
	}
	raw := bytes.TrimSpace(params)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		raw = []byte("{}")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return &ValidationError{Details: []FieldError{{
			Field:   "",
			Message: "Параметры должны быть JSON-объектом. Проверьте тело запроса.",
		}}}
	}
	fields := Fields(s)
	details := make([]FieldError, 0)
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		seen[f.ID] = struct{}{}
		val, ok := obj[f.ID]
		if !ok {
			if f.Required {
				message := fmt.Sprintf("Нет поля «%s». Заполните его.", f.Label)
				if f.AllowUnknown {
					message = fmt.Sprintf("Нет поля «%s». Заполните его; если значение неизвестно, оставьте поле пустым.", f.Label)
				}
				details = append(details, FieldError{
					Field:   f.ID,
					Message: message,
				})
			}
			continue
		}
		if err := checkField(f, val); err != nil {
			details = append(details, *err)
		}
	}
	extra := make([]string, 0)
	for k := range obj {
		if _, ok := seen[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		details = append(details, FieldError{
			Field:   k,
			Message: fmt.Sprintf("Лишнее поле %s. Удалите его из запроса.", k),
		})
	}
	if len(details) > 0 {
		return &ValidationError{Details: details}
	}
	return nil
}

// CheckKeys validates the given keys of params for the object type, as Validate does for the whole object.
// keys nil checks every key. It returns the first failing key, or "" when the object itself is malformed.
func CheckKeys(objectType string, params json.RawMessage, keys []string) (string, bool) {
	if keys == nil {
		err := Validate(objectType, params)
		if err == nil {
			return "", true
		}
		var ve *ValidationError
		if errors.As(err, &ve) && len(ve.Details) > 0 {
			return ve.Details[0].Field, false
		}
		return "", false
	}
	s, err := SchemaFor(objectType)
	if err != nil {
		return "", false
	}
	obj, err := paramsObject(params)
	if err != nil {
		return "", false
	}
	fields := map[string]Field{}
	for _, f := range Fields(s) {
		fields[f.ID] = f
	}
	for _, k := range keys {
		f, known := fields[k]
		if !known {
			return k, false
		}
		val, present := obj[k]
		if !present {
			if f.Required {
				return k, false
			}
			continue
		}
		if checkField(f, val) != nil {
			return k, false
		}
	}
	return "", true
}

func paramsObject(params json.RawMessage) (map[string]json.RawMessage, error) {
	raw := bytes.TrimSpace(params)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		raw = []byte("{}")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, &ValidationError{Details: []FieldError{{
			Field:   "",
			Message: "Параметры должны быть JSON-объектом. Проверьте тело запроса.",
		}}}
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	return obj, nil
}

func rewriteParams(objectType string, params json.RawMessage, fill bool) (json.RawMessage, error) {
	s, err := SchemaFor(objectType)
	if err != nil {
		return nil, err
	}
	obj, err := paramsObject(params)
	if err != nil {
		return nil, err
	}
	for _, f := range Fields(s) {
		val, ok := obj[f.ID]
		if !ok {
			if !fill {
				continue
			}
			b, err := json.Marshal(f.Default)
			if err != nil {
				return nil, fmt.Errorf("objects.defaults.%s: %w", f.ID, err)
			}
			obj[f.ID] = b
			continue
		}
		switch f.Type {
		case TypeTimeRange:
			if next, changed := rewriteTimeRange(val); changed {
				obj[f.ID] = next
			}
		case TypeDimensions:
			if next, changed := rewriteDimensions(val); changed {
				obj[f.ID] = next
			}
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("objects.rewrite: %w", err)
	}
	return out, nil
}

// FillDefaults copies missing schema fields from defaults and normalizes values as NormalizeParams does.
func FillDefaults(objectType string, params json.RawMessage) (json.RawMessage, error) {
	return rewriteParams(objectType, params, true)
}

// NormalizeParams rewrites time_range values to HH:MM and "LxWxH" strings to Dimensions. Missing keys stay missing.
func NormalizeParams(objectType string, params json.RawMessage) (json.RawMessage, error) {
	return rewriteParams(objectType, params, false)
}

func rewriteTimeRange(raw json.RawMessage) (json.RawMessage, bool) {
	trim := bytes.TrimSpace(raw)
	if bytes.Equal(trim, []byte("null")) {
		return raw, false
	}
	var tr TimeRange
	if err := json.Unmarshal(raw, &tr); err != nil {
		return raw, false
	}
	start, ok1 := NormalizeClock(tr.Start)
	end, ok2 := NormalizeClock(tr.End)
	if !ok1 || !ok2 {
		return raw, false
	}
	if start == tr.Start && end == tr.End {
		return raw, false
	}
	tr.Start = start
	tr.End = end
	b, err := json.Marshal(tr)
	if err != nil {
		return raw, false
	}
	return b, true
}

// NOTE: projects saved before the dimensions type kept sizes as "1200x800x1600" strings.
func rewriteDimensions(raw json.RawMessage) (json.RawMessage, bool) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return raw, false
	}
	d, ok := ParseDimensions(text)
	if !ok {
		return raw, false
	}
	b, err := json.Marshal(d)
	if err != nil {
		return raw, false
	}
	return b, true
}

// ParseDimensions reads "1200x800x1600"; the separator may also be X, the multiplication sign or Cyrillic х.
func ParseDimensions(text string) (Dimensions, bool) {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == 'x' || r == 'X' || r == '\u00d7' || r == '\u0445' || r == '\u0425' || r == '*'
	})
	if len(parts) != 3 {
		return Dimensions{}, false
	}
	var v [3]float64
	for k, p := range parts {
		n, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(p), ",", "."), 64)
		if err != nil || !(n > 0) || math.IsInf(n, 0) {
			return Dimensions{}, false
		}
		v[k] = n
	}
	return Dimensions{Length: v[0], Width: v[1], Height: v[2]}, true
}

func decodeDimensions(raw json.RawMessage) (Dimensions, bool) {
	var parts struct {
		Length *float64 `json:"length"`
		Width  *float64 `json:"width"`
		Height *float64 `json:"height"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parts); err != nil || parts.Length == nil || parts.Width == nil || parts.Height == nil {
		return Dimensions{}, false
	}
	return Dimensions{Length: *parts.Length, Width: *parts.Width, Height: *parts.Height}, true
}

// NormalizeClock parses HH:MM, H:MM, HH:MM:SS, or HH:MM:SS.sss into HH:MM.
func NormalizeClock(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return "", false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return "", false
	}
	if len(parts) == 3 {
		sec, err := strconv.Atoi(parts[2])
		if err != nil || sec < 0 || sec > 59 {
			return "", false
		}
	}
	return fmt.Sprintf("%02d:%02d", h, m), true
}

func checkField(f Field, raw json.RawMessage) *FieldError {
	trim := bytes.TrimSpace(raw)
	if bytes.Equal(trim, []byte("null")) {
		if f.AllowUnknown {
			return nil
		}
		return &FieldError{
			Field:   f.ID,
			Message: fmt.Sprintf("Поле «%s» не должно быть пустым. Укажите значение.", f.Label),
		}
	}
	switch f.Type {
	case TypeString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть строкой. Исправьте тип значения.", f.Label),
			}
		}
		if strings.TrimSpace(s) == "" {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» не должно быть пустым. Укажите значение.", f.Label),
			}
		}
		return nil
	case TypeBoolean:
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть Да, Нет или Неизвестно.", f.Label),
			}
		}
		return nil
	case TypeEnum:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть одним из списка. Выберите значение.", f.Label),
			}
		}
		if !optionOK(f, s) {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» имеет недопустимое значение. Выберите пункт из списка.", f.Label),
			}
		}
		return nil
	case TypeMultiEnum:
		var vals []string
		if err := json.Unmarshal(raw, &vals); err != nil {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть списком значений. Отметьте нужные пункты.", f.Label),
			}
		}
		if f.Required && len(vals) == 0 && !f.AllowUnknown {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» не должно быть пустым. Отметьте хотя бы один пункт.", f.Label),
			}
		}
		for _, v := range vals {
			if !optionOK(f, v) {
				return &FieldError{
					Field:   f.ID,
					Message: fmt.Sprintf("Поле «%s» содержит недопустимое значение. Оставьте только пункты из списка.", f.Label),
				}
			}
		}
		return nil
	case TypeTimeRange:
		var tr TimeRange
		if err := json.Unmarshal(raw, &tr); err != nil {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть интервалом времени ЧЧ:ММ. Исправьте значение.", f.Label),
			}
		}
		if !validClock(tr.Start) || !validClock(tr.End) {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть интервалом ЧЧ:ММ. Пример: 08:00 и 22:00.", f.Label),
			}
		}
		return nil
	case TypeDimensions:
		d, ok := decodeDimensions(raw)
		if !ok {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно задавать длину, ширину и высоту числами. Заполните все три.", f.Label),
			}
		}
		for _, part := range []struct {
			name string
			v    float64
		}{{"длина", d.Length}, {"ширина", d.Width}, {"высота", d.Height}} {
			if (f.Min != nil && part.v < *f.Min) || (f.Max != nil && part.v > *f.Max) {
				return &FieldError{
					Field: f.ID,
					Message: fmt.Sprintf("Поле «%s»: %s %s %s вне диапазона. Укажите число от %s до %s.",
						f.Label, part.name, fmtNum(part.v), f.Unit, fmtNum(derefMin(f)), fmtNum(derefMax(f))),
				}
			}
		}
		return nil
	case TypeNumber, TypeInteger:
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть числом. Исправьте тип значения.", f.Label),
			}
		}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть конечным числом. Исправьте значение.", f.Label),
			}
		}
		if f.Type == TypeInteger && n != math.Trunc(n) {
			return &FieldError{
				Field:   f.ID,
				Message: fmt.Sprintf("Поле «%s» должно быть целым числом. Укажите целое значение.", f.Label),
			}
		}
		if f.Min != nil && n < *f.Min {
			return &FieldError{
				Field: f.ID,
				Message: fmt.Sprintf("Значение поля «%s» меньше минимума %s. Укажите число от %s до %s.",
					f.Label, fmtNum(*f.Min), fmtNum(*f.Min), fmtNum(derefMax(f))),
			}
		}
		if f.Max != nil && n > *f.Max {
			return &FieldError{
				Field: f.ID,
				Message: fmt.Sprintf("Значение поля «%s» больше максимума %s. Укажите число от %s до %s.",
					f.Label, fmtNum(*f.Max), fmtNum(derefMin(f)), fmtNum(*f.Max)),
			}
		}
		return nil
	default:
		return &FieldError{
			Field:   f.ID,
			Message: fmt.Sprintf("Поле «%s» имеет неизвестный тип схемы. Сообщите идентификатор запроса.", f.Label),
		}
	}
}

func derefMin(f Field) float64 {
	if f.Min == nil {
		return 0
	}
	return *f.Min
}

func derefMax(f Field) float64 {
	if f.Max == nil {
		return 0
	}
	return *f.Max
}

func fmtNum(v float64) string {
	return rutext.Num(v, 4)
}

func n(id, label, unit, note string, def, min, max float64) Field {
	mn, mx := min, max
	return Field{
		ID: id, Label: label, Unit: unit, Type: TypeNumber, Required: true,
		Default: def, Min: &mn, Max: &mx, Note: note,
	}
}

func i(id, label, unit, note string, def, min, max float64) Field {
	f := n(id, label, unit, note, def, min, max)
	f.Type = TypeInteger
	return f
}

func s(id, label, unit, note, def string) Field {
	return Field{
		ID: id, Label: label, Unit: unit, Type: TypeString, Required: true,
		Default: def, Note: note,
	}
}

func b(id, label, note string, def bool, allowUnknown bool) Field {
	return Field{
		ID: id, Label: label, Unit: "-", Type: TypeBoolean, Required: true,
		AllowUnknown: allowUnknown, Default: def, Note: note,
	}
}

func e(id, label, note, def string, values []string) Field {
	opts := make([]EnumOption, 0, len(values))
	for _, v := range values {
		opts = append(opts, EnumOption{Value: v, Label: v})
	}
	return Field{
		ID: id, Label: label, Unit: "-", Type: TypeEnum, Required: true,
		Default: def, Note: note, Options: opts,
	}
}

func me(id, label, note string, def []string, opts []EnumOption) Field {
	return Field{
		ID: id, Label: label, Unit: "-", Type: TypeMultiEnum, Required: true,
		Default: def, Note: note, Options: opts,
	}
}

func tr(id, label, note, start, end string) Field {
	return Field{
		ID: id, Label: label, Unit: "чч:мм", Type: TypeTimeRange, Required: true,
		Default: TimeRange{Start: start, End: end}, Note: note,
	}
}

func dims(id, label, note string, def Dimensions, min, max float64) Field {
	mn, mx := min, max
	return Field{
		ID: id, Label: label, Unit: "мм", Type: TypeDimensions, Required: true,
		Default: def, Min: &mn, Max: &mx, Note: note,
	}
}

func allowUnknown(f Field) Field {
	f.AllowUnknown = true
	return f
}

// optional is a limit the site may not know. It has no default and may be absent, so projects saved before the
// field existed stay valid; a matching rule that needs it is skipped while it is empty.
func optional(id, label, unit, note string, min, max float64) Field {
	f := allowUnknown(n(id, label, unit, note, 0, min, max))
	f.Required = false
	f.Default = nil
	return f
}

// withShort sets the one-line labels of a schema's fields from a map by field id.
func withShort(s Schema, short map[string]string) Schema {
	for gi := range s.Groups {
		for fi := range s.Groups[gi].Fields {
			f := &s.Groups[gi].Fields[fi]
			if v, ok := short[f.ID]; ok && v != f.Label {
				f.Short = v
			}
		}
	}
	return s
}

// NOTE: aliases keep label matching for organizer files that were written before a label was reworded.
func alias(f Field, labels ...string) Field {
	f.Aliases = append(f.Aliases, labels...)
	return f
}

func optionOK(f Field, v string) bool {
	for _, o := range f.Options {
		if o.Value == v {
			return true
		}
	}
	return false
}

func validClock(s string) bool {
	_, ok := NormalizeClock(s)
	return ok
}
