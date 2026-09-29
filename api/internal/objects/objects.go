package objects

const (
	Warehouse = "warehouse"
	Airport   = "airport"
	Hospital  = "hospital"
)

func Valid(t string) bool {
	switch t {
	case Warehouse, Airport, Hospital:
		return true
	default:
		return false
	}
}
