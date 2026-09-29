package importers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func field(t *testing.T, code string) Field {
	t.Helper()
	f, ok := FieldByCode(code)
	if !ok {
		t.Fatalf("no field %q", code)
	}
	return f
}

func TestFieldParse(t *testing.T) {
	tests := []struct {
		code, cell string
		want       any
		wantErr    string
	}{
		{code: "price_rub", cell: "2 700 000,00", want: 2700000.0},
		{code: "price_rub", cell: "2\u00a0700\u00a0000 ₽", want: 2700000.0},
		{code: "price_rub", cell: "950000 руб.", want: 950000.0},
		{code: "price_rub", cell: "-5", wantErr: "Цена: число от 0 до"},
		{code: "payload_kg", cell: "1 500", want: 1500.0},
		{code: "payload_kg", cell: "2,5", want: 2.5},
		{code: "payload_kg", cell: "2.5 кг", want: 2.5},
		{code: "payload_kg", cell: "много", wantErr: "Грузоподъёмность: нужно число, а в ячейке «много»."},
		{code: "temp_min_c", cell: "\u221220", want: -20.0},
		{code: "service_pct_year", cell: "5 %", want: 0.05},
		{code: "service_pct_year", cell: "12,5", want: 0.125},
		{code: "service_pct_year", cell: "150", wantErr: "Сервис в год: число от 0 до 100%."},
		{code: "ugt", cell: "7", want: 7.0},
		{code: "ugt", cell: "3,5", wantErr: "УГТ: нужно целое число."},
		{code: "ugt", cell: "10", wantErr: "УГТ: число от 1 до 9."},
		{code: "family", cell: "Мобильные роботы", want: "mobile"},
		{code: "family", cell: "Роботы-манипуляторы", want: "manipulator"},
		{code: "family", cell: "дроны", want: "uav"},
		{code: "family", cell: "Экзоскелет", wantErr: "Тип: нет такого значения: «Экзоскелет»."},
		{code: "kind", cell: "БАС", want: "bas"},
		{code: "kind", cell: "brs", want: "brs"},
		{code: "status", cell: "Пилот", want: "piloting"},
		{code: "status", cell: "operation", want: "operation"},
		{code: "confidence", cell: "Данные производителя", want: "vendor"},
		{code: "object_types", cell: "Больница; склад, Склад", want: []string{"warehouse", "hospital"}},
		{code: "object_types", cell: "Завод", wantErr: "Объекты: нет такого значения: «Завод»."},
		{code: "source_url", cell: "https://example.com/robot", want: "https://example.com/robot"},
		{code: "source_url", cell: "example.com", wantErr: "Источник: нужна ссылка, которая начинается с http:// или https://."},
		{code: "source_url", cell: "javascript:alert(1)", wantErr: "Источник: нужна ссылка"},
		{code: "sourced_at", cell: "28.09.2026", want: "2026-09-28"},
		{code: "sourced_at", cell: "2026-09-28", want: "2026-09-28"},
		{code: "sourced_at", cell: "01.01.1970", wantErr: "Дата источника: нужна дата в виде 28.09.2026"},
		{code: "id", cell: "5760E938-9A43-45A7-B8E8-F4F2E6383930", want: "5760e938-9a43-45a7-b8e8-f4f2e6383930"},
		{code: "id", cell: "робот-1", wantErr: "Идентификатор: нужен идентификатор из выгрузки каталога"},
		{code: "name", cell: `  Штабелёр "Альфа"  `, want: "Штабелёр «Альфа»"},
		{code: "cases", cell: "с 40\u00a0% до 100\u00a0%", want: "с 40\u00a0% до 100\u00a0%"},
		{code: "name", cell: strings.Repeat("я", 301), wantErr: "Название: не длиннее 300 знаков, а сейчас 301."},
		{code: "payload_kg", cell: "   ", want: nil},
	}
	for _, tc := range tests {
		got, err := field(t, tc.code).Parse(tc.cell)
		if tc.wantErr != "" {
			var fe *FieldError
			if !errors.As(err, &fe) || !strings.HasPrefix(fe.Message, strings.TrimSuffix(tc.wantErr, ".")) {
				t.Errorf("%s %q: error %v, want %q", tc.code, tc.cell, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s %q: %v", tc.code, tc.cell, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %q = %#v, want %#v", tc.code, tc.cell, got, tc.want)
		}
	}
}

func TestFieldFormatParsesBack(t *testing.T) {
	values := map[string]any{
		"name": "Штабелёр «Альфа»", "family": "stationary", "kind": "bas", "status": "rnd", "ugt": 6.0,
		"market": 7.5, "price_rub": 12345678.9, "object_types": []string{"airport", "hospital"},
		"source_url": "https://example.com/a?b=1", "confidence": "assumed", "sourced_at": "2026-01-31",
		"payload_kg": 0.25, "temp_min_c": -30.0, "service_pct_year": 0.075, "nav_type": "Лидар",
	}
	for code, v := range values {
		f := field(t, code)
		got, err := f.Parse(f.Format(v))
		if err != nil {
			t.Errorf("%s: %q does not parse: %v", code, f.Format(v), err)
			continue
		}
		if !Equal(code, got, v) {
			t.Errorf("%s: %#v -> %q -> %#v", code, v, f.Format(v), got)
		}
	}
}

func TestFieldFromJSON(t *testing.T) {
	tests := []struct {
		code, raw string
		want      any
		wantErr   bool
	}{
		{code: "payload_kg", raw: "null", want: nil},
		{code: "payload_kg", raw: "1500", want: 1500.0},
		{code: "payload_kg", raw: `"1500"`, wantErr: true},
		{code: "service_pct_year", raw: "0.05", want: 0.05},
		{code: "service_pct_year", raw: "5", wantErr: true},
		{code: "object_types", raw: `["hospital","warehouse"]`, want: []string{"warehouse", "hospital"}},
		{code: "object_types", raw: `[]`, want: nil},
		{code: "object_types", raw: `["factory"]`, wantErr: true},
		{code: "family", raw: `"humanoid"`, want: "humanoid"},
		{code: "family", raw: `"Антропоморфные роботы"`, wantErr: true},
		{code: "name", raw: `"  "`, want: nil},
		{code: "sourced_at", raw: `"2026-09-28"`, want: "2026-09-28"},
	}
	for _, tc := range tests {
		got, err := field(t, tc.code).FromJSON(json.RawMessage(tc.raw))
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s %s: want error, got %#v", tc.code, tc.raw, got)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %s = %#v, %v; want %#v", tc.code, tc.raw, got, err, tc.want)
		}
	}
}

func TestFieldForHeader(t *testing.T) {
	tests := map[string]string{
		"тип":                  "kind",
		"Тип":                  "family",
		"Название":             "name",
		"наименование":         "name",
		"Грузоподъёмность, кг": "payload_kg",
		"грузоподъемность":     "payload_kg",
		"Цена изделия":         "price_rub",
		"Цена, ₽":              "price_rub",
		"Сервис в год, %":      "service_pct_year",
		"Рын Потенциал":        "market",
		"Кейсы":                "cases",
		"Выгрузка":             StampField,
		"id":                   "id",
		"Фото":                 "photo",
		"Вес, кг":              "mass_kg",
		"Цена изделия (руб.)":  "price_rub",
		"Время работы, ч, max": "endurance_h",
	}
	for header, want := range tests {
		f, ok := FieldForHeader(header)
		if !ok || f.Code != want {
			t.Errorf("FieldForHeader(%q) = %q, %v; want %q", header, f.Code, ok, want)
		}
	}
	for _, header := range []string{"", "Примечание", "Цвет корпуса"} {
		if f, ok := FieldForHeader(header); ok {
			t.Errorf("FieldForHeader(%q) = %q, want none", header, f.Code)
		}
	}
}

func TestFieldsHaveUniqueCodesAndHeaders(t *testing.T) {
	codes := map[string]bool{}
	for _, f := range Fields() {
		if codes[f.Code] {
			t.Errorf("code %q twice", f.Code)
		}
		codes[f.Code] = true
		got, ok := FieldForHeader(f.Header())
		if !ok || got.Code != f.Code {
			t.Errorf("header %q of %q maps to %q", f.Header(), f.Code, got.Code)
		}
		if f.Kind == FieldNumber && (f.Min == nil || f.Max == nil) {
			t.Errorf("number field %q has no range", f.Code)
		}
	}
}
