package sms

import (
	smsint "github.com/imbytecat/moonbase/integrations/sms"
	"github.com/imbytecat/moonbase/integrations/sms/aliyun"
	"github.com/imbytecat/moonbase/integrations/sms/tencent"
)

func NewRegistry() smsint.Registry { return smsint.MustRegistry(aliyun.New(), tencent.New()) }
