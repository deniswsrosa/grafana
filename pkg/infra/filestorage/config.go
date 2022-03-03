package filestorage

type commonBackendConfig struct {
	Name                string      `json:"name"`
	AllowedPrefixes     []string    `json:"allowedPrefixes"`     // null -> all paths are allowed
	SupportedOperations []Operation `json:"supportedOperations"` // null -> all operations are supported
}

type fsBackendConfig struct {
	commonBackendConfig
	Path string `json:"path"`
}

type dbBackendConfig struct {
	commonBackendConfig
}

type backendsConfig struct {
	FS []fsBackendConfig `json:"fs"`
	DB []dbBackendConfig `json:"db"`
}

type filestorageConfig struct {
	Backends backendsConfig `json:"backends"`
}

func newConfig(staticRootPath string) filestorageConfig {
	return filestorageConfig{
		Backends: backendsConfig{
			FS: []fsBackendConfig{
				{
					commonBackendConfig: commonBackendConfig{
						Name: "public",
						AllowedPrefixes: []string{
							"testdata/",
							"img/icons/",
							"img/bg/",
							"gazetteer/",
							"maps/",
							"upload/",
						},
						SupportedOperations: []Operation{
							OperationListFiles, OperationListFolders,
						},
					},
					Path: staticRootPath,
				},
			},
			DB: make([]dbBackendConfig, 0),
		},
	}
}
