package globals

func IsSubset(smaller, larger map[string]string) bool {
	for key, value := range smaller {
		if largerValue, ok := larger[key]; !ok || largerValue != value {
			return false
		}
	}
	return true
}
