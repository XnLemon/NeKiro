package workspace

func NewOwnerPolicy() Authorizer {
	return OwnerPolicy{}
}
