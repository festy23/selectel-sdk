// Code generated from current request schema; DO NOT EDIT.
package dns

const (
	minTTL           = 60
	maxTTL           = 604800
	minRecords       = 1
	maxRecords       = 100
	maxCommentLength = 255
)

var requestRecordTypes = map[AllowedRecordTypes]bool{"A": true, "AAAA": true, "ALIAS": true, "CAA": true, "CNAME": true, "DNAME": true, "HTTPS": true, "MX": true, "NS": true, "SOA": true, "SRV": true, "SVCB": true, "SSHFP": true, "TXT": true}
