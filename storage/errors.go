package storage

import "errors"

var (
	ErrGet      = errors.New("unable to retrieve data from cache storage")
	ErrSet      = errors.New("unable to store data in cache storage")
	ErrGetMulti = errors.New("unable to retrieve multiple entries from cache storage")
	ErrSetMulti = errors.New("unable to store multiple entries in cache storage")
)
