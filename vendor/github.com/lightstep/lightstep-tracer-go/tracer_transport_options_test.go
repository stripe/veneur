package lightstep_test

// Options

func toTestOptions(options []testOption) testOptions {
	structuredOptions := testOptions{}

	for _, option := range options {
		option.apply(&structuredOptions)
	}

	return structuredOptions
}

type testOptions struct {
	supportsBaggage     bool
	supportsReference   bool
	supportsTypedValues bool
}

type testOption interface {
	apply(options *testOptions)
}

func thatSupportsBaggage() testOption {
	return baggageOption{}
}

type baggageOption struct{}

func (baggageOption) apply(options *testOptions) {
	options.supportsBaggage = true
}

func thatSupportsReference() testOption {
	return supportsReferences{}
}

type supportsReferences struct{}

func (supportsReferences) apply(options *testOptions) {
	options.supportsReference = true
}

func thatSupportsTypedValues() testOption {
	return supportsTypedValues{}
}

type supportsTypedValues struct{}

func (supportsTypedValues) apply(options *testOptions) {
	options.supportsTypedValues = true
}
