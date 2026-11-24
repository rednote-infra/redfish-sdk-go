package models

import "encoding/json"

type CertificateType string

const (
	CertificatePEM   CertificateType = "PEM"
	CertificatePKCS7 CertificateType = "PKCS7"
)

// SecureBootDatabase advertises enrollment collections for one UEFI key database.
type SecureBootDatabase struct {
	ODataID      *string                    `json:"@odata.id,omitempty"`
	ID           *StringOrNumber            `json:"Id,omitempty"`
	Name         *string                    `json:"Name,omitempty"`
	DatabaseID   *string                    `json:"DatabaseId,omitempty"`
	Certificates *Link                      `json:"Certificates,omitempty"`
	Signatures   *Link                      `json:"Signatures,omitempty"`
	Actions      map[string]ActionTarget    `json:"Actions,omitempty"`
	Extra        map[string]json.RawMessage `json:"-"`
	NullFields   map[string]bool            `json:"-"`
}

func (m *SecureBootDatabase) UnmarshalJSON(data []byte) error {
	type plain SecureBootDatabase
	extra, err := decodeExtra(data, (*plain)(m))
	m.Extra = extra
	return err
}
func (m SecureBootDatabase) MarshalJSON() ([]byte, error) {
	type plain SecureBootDatabase
	return encodeExtra(plain(m), m.Extra)
}
func (m SecureBootDatabase) ResourceID() string  { return string(Value(m.ID)) }
func (m SecureBootDatabase) ResourceURI() string { return Value(m.ODataID) }

// SecureBootCertificateRequest enrolls a certificate in the advertised collection.
type SecureBootCertificateRequest struct {
	CertificateString  string          `json:"CertificateString"`
	CertificateType    CertificateType `json:"CertificateType"`
	UEFISignatureOwner *string         `json:"UefiSignatureOwner,omitempty"`
}

// SecureBootSignatureRequest enrolls a hash/signature in a UEFI database.
type SecureBootSignatureRequest struct {
	SignatureString       string  `json:"SignatureString"`
	SignatureTypeRegistry string  `json:"SignatureTypeRegistry"`
	SignatureType         string  `json:"SignatureType"`
	UefiSignatureOwner    *string `json:"UefiSignatureOwner,omitempty"`
}
