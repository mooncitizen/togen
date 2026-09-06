package ir

type EngineInfo struct {
	Port     int
	Versions []string
}

var Engines = map[CloudProvider]map[Engine]EngineInfo{
	ProviderAWS: {
		EnginePostgres: {Port: 5432, Versions: []string{"17", "16", "15"}},
		EngineMySQL:    {Port: 3306, Versions: []string{"8.4", "8.0"}},
	},
	ProviderGCP: {
		EnginePostgres: {Port: 5432, Versions: []string{"17", "16", "15"}},
		EngineMySQL:    {Port: 3306, Versions: []string{"8.4", "8.0"}},
	},
}

func DefaultEngineVersion(p CloudProvider, e Engine) (string, bool) {
	info, ok := Engines[p][e]
	if !ok || len(info.Versions) == 0 {
		return "", false
	}
	return info.Versions[0], true
}
