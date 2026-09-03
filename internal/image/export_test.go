package image

// GetFilteredList exposes the build-field filter to the external test package.
// BuildAndPush is the only caller and it shells out to docker immediately
// afterwards, so this is the only way to assert on the filter alone.
func (d *Docker) GetFilteredList(
	services []ServiceProperties,
) []ServiceProperties {
	return d.getFilteredList(services)
}

// BakeArgs exposes the argv handed to docker, which is otherwise only
// observable by running a real build.
func (d *Docker) BakeArgs() []string {
	return d.bakeArgs()
}
