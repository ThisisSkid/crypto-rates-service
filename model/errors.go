package model

import "errors"

// ErrNotFound — в таблице нет подходящей строки. Это не обрыв связи с базой.
var ErrNotFound = errors.New("снимок не найден")
