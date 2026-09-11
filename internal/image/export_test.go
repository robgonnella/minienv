package image

// GetFilteredList exposes the build-field filter, which is otherwise reachable
// only through a code path that shells out to docker.
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
