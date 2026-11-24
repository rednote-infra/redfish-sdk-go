package models

// Object is an explicitly constructed mutation or a genuinely dynamic resource.
// Nil values send JSON null; omitted keys are never synthesized.
type Object map[string]any
