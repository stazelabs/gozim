package zim

// Item provides access to content data for non-redirect entries.
type Item struct {
	entry Entry
}

// Data reads and returns the blob content for this item.
func (i Item) Data() (Blob, error) {
	c, err := i.entry.archive.readCluster(i.entry.clusterNum)
	if err != nil {
		return Blob{}, err
	}
	if int(i.entry.blobNum) >= len(c.blobs) {
		return Blob{}, ErrNotFound
	}
	return Blob{data: c.blobs[i.entry.blobNum]}, nil
}

// MIMEType returns the MIME type of this item.
func (i Item) MIMEType() string { return i.entry.MIMEType() }

// Path returns the path of this item.
func (i Item) Path() string { return i.entry.Path() }

// FullPath returns the full path including namespace.
func (i Item) FullPath() string { return i.entry.FullPath() }
