package storage

import (
	storageint "github.com/imbytecat/moonbase/integrations/storage"
	"github.com/imbytecat/moonbase/integrations/storage/local"
	"github.com/imbytecat/moonbase/integrations/storage/s3"
)

func NewRegistry() storageint.Registry { return storageint.MustRegistry(s3.New(), local.New()) }
