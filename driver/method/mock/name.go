package mock

import (
	"authorization"
)

func (m *Method) Name() authorization.MethodID { return m.id }
