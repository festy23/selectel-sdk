package selectelsdk

// Generation uses only pinned public artifacts and never accesses the cloud.
//go:generate python3 internal/imagecontractgen/generate.py
//go:generate python3 internal/modelgen/generate.py
//go:generate go run ./internal/cataloggen
