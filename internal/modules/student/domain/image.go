package domain

const MaxImageBytes = 5 * 1024 * 1024

type Image struct {
	Key                 string
	ContentType         string
	Size, Width, Height int
}
