package mock

import (
	"github.com/not-for-prod/authorization"
)

func (m *Method) Name() authorization.MethodID { return m.id }
