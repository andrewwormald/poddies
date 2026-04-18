package config

import "fmt"

// Adapter identifies which CLI a member uses.
type Adapter string

const (
	AdapterClaude Adapter = "claude"
	AdapterGemini Adapter = "gemini"
	// AdapterMock is used in tests so adapter validation can succeed without
	// a real CLI on the PATH.
	AdapterMock Adapter = "mock"
)

// ValidAdapters lists adapters recognized by the validator. Adding a new
// real adapter means updating this list and the registry in internal/adapter.
var ValidAdapters = []Adapter{AdapterClaude, AdapterGemini, AdapterMock}

// Validate checks a is a known adapter.
func (a Adapter) Validate() error {
	for _, v := range ValidAdapters {
		if a == v {
			return nil
		}
	}
	return fmt.Errorf("unknown adapter %q (valid: %v)", a, ValidAdapters)
}

// Effort is a coarse per-adapter knob mapped into native flags (e.g. Claude
// thinking budget, Gemini reasoning_effort).
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// ValidEfforts enumerates the allowed Effort values.
var ValidEfforts = []Effort{EffortLow, EffortMedium, EffortHigh}

// Validate checks e is a known effort level.
func (e Effort) Validate() error {
	for _, v := range ValidEfforts {
		if e == v {
			return nil
		}
	}
	return fmt.Errorf("unknown effort %q (valid: %v)", e, ValidEfforts)
}

// Trigger controls when the chief-of-staff agent runs.
type Trigger string

const (
	TriggerUnresolvedRouting Trigger = "unresolved_routing"
	TriggerMilestone         Trigger = "milestone"
	TriggerHumanAsk          Trigger = "human_ask"
	TriggerDrift             Trigger = "drift"
)

// ValidTriggers enumerates the allowed Trigger values.
var ValidTriggers = []Trigger{
	TriggerUnresolvedRouting,
	TriggerMilestone,
	TriggerHumanAsk,
	TriggerDrift,
}

// Validate checks t is a known trigger.
func (t Trigger) Validate() error {
	for _, v := range ValidTriggers {
		if t == v {
			return nil
		}
	}
	return fmt.Errorf("unknown trigger %q (valid: %v)", t, ValidTriggers)
}
