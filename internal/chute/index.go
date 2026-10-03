package chute

type Index struct {
	Resources []Resource
	ByKind    map[string][]Resource
	ByName    map[string][]Resource
}

func NewIndex(resources []Resource) *Index {
	index := &Index{
		Resources: resources,
		ByKind:    make(map[string][]Resource),
		ByName:    make(map[string][]Resource),
	}

	for _, resource := range resources {
		index.ByKind[resource.Kind] = append(index.ByKind[resource.Kind], resource)
		index.ByName[resource.Name] = append(index.ByName[resource.Name], resource)
	}

	return index
}

func (i *Index) Kind(kind string) []Resource {
	return i.ByKind[kind]
}

func (i *Index) Longhorn(kind string) []Resource {
	var out []Resource
	for _, resource := range i.Kind(kind) {
		if resource.Longhorn() {
			out = append(out, resource)
		}
	}
	return out
}

func (i *Index) LonghornVolumes() []Resource {
	return i.Longhorn("Volume")
}

func (i *Index) Get(kind, name, namespace string) *Resource {
	for _, resource := range i.Kind(kind) {
		if resource.Name == name && (namespace == "" || resource.Namespace == namespace) {
			copy := resource
			return &copy
		}
	}
	return nil
}
