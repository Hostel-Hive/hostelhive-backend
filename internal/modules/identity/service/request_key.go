package service

func ValidRequestKey(key string) bool { return keyPattern.MatchString(key) }
