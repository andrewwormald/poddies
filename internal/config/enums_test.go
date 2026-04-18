package config

import "testing"

func TestAdapter_Validate_KnownValues(t *testing.T) {
	for _, a := range ValidAdapters {
		if err := a.Validate(); err != nil {
			t.Errorf("%q should be valid, got %v", a, err)
		}
	}
}

func TestAdapter_Validate_UnknownErrors(t *testing.T) {
	if err := Adapter("nonsense").Validate(); err == nil {
		t.Error("want error for unknown adapter")
	}
}

func TestAdapter_Validate_EmptyErrors(t *testing.T) {
	if err := Adapter("").Validate(); err == nil {
		t.Error("want error for empty adapter")
	}
}

func TestEffort_Validate_KnownValues(t *testing.T) {
	for _, e := range ValidEfforts {
		if err := e.Validate(); err != nil {
			t.Errorf("%q should be valid, got %v", e, err)
		}
	}
}

func TestEffort_Validate_UnknownErrors(t *testing.T) {
	if err := Effort("extreme").Validate(); err == nil {
		t.Error("want error for unknown effort")
	}
}

func TestEffort_Validate_EmptyErrors(t *testing.T) {
	if err := Effort("").Validate(); err == nil {
		t.Error("want error for empty effort")
	}
}

func TestTrigger_Validate_KnownValues(t *testing.T) {
	for _, tr := range ValidTriggers {
		if err := tr.Validate(); err != nil {
			t.Errorf("%q should be valid, got %v", tr, err)
		}
	}
}

func TestTrigger_Validate_UnknownErrors(t *testing.T) {
	if err := Trigger("bogus").Validate(); err == nil {
		t.Error("want error for unknown trigger")
	}
}
