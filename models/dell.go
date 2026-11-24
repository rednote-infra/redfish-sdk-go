package models

import "fmt"

// DellServiceStatus preserves both documented and future iDRAC status values.
type DellServiceStatus string

const (
	DellStatusReady               DellServiceStatus = "Ready"
	DellStatusNotReady            DellServiceStatus = "NotReady"
	DellStatusInUse               DellServiceStatus = "InUse"
	DellStatusReloadingData       DellServiceStatus = "ReloadingData"
	DellStatusDisabled            DellServiceStatus = "Disabled"
	DellStatusInRecovery          DellServiceStatus = "InRecovery"
	DellStatusNotInitialized      DellServiceStatus = "NotInitialized"
	DellStatusPoweredOff          DellServiceStatus = "PoweredOff"
	DellStatusInPOST              DellServiceStatus = "InPOST"
	DellStatusOutOfPOST           DellServiceStatus = "OutOfPOST"
	DellStatusCollectingInventory DellServiceStatus = "CollectingSystemInventory"
	DellStatusAutomatedTask       DellServiceStatus = "AutomatedTaskExecution"
	DellStatusLCUSC               DellServiceStatus = "LCUSC"
	DellStatusF2Setup             DellServiceStatus = "EnteredF2Setup"
	DellStatusF11BootManager      DellServiceStatus = "EnteredF11BootManager"
	DellStatusHaltedF1F2          DellServiceStatus = "HaltedF1/F2Prompt"
	DellStatusHaltedF1F2F11       DellServiceStatus = "HaltedF1/F2/F11Prompt"
)

// DellRemoteServicesAPIStatus is the GetRemoteServicesAPIStatus action response.
type DellRemoteServicesAPIStatus struct {
	Status                 DellServiceStatus `json:"Status"`
	LCStatus               DellServiceStatus `json:"LCStatus"`
	RTStatus               DellServiceStatus `json:"RTStatus"`
	ServerStatus           DellServiceStatus `json:"ServerStatus"`
	SEKMServiceStatus      DellServiceStatus `json:"SEKMServiceStatus"`
	TelemetryServiceStatus DellServiceStatus `json:"TelemetryServiceStatus"`
	ExtendedInfo           []ExtendedInfo    `json:"@Message.ExtendedInfo,omitempty"`
}

func (s DellRemoteServicesAPIStatus) IsReady() bool { return s.Status == DellStatusReady }

type DellExportFormat string
type DellExportUse string
type DellExportInclude string
type DellShareType string

const (
	DellExportXML              DellExportFormat  = "XML"
	DellExportJSON             DellExportFormat  = "JSON"
	DellExportDefault          DellExportUse     = "Default"
	DellExportClone            DellExportUse     = "Clone"
	DellExportReplace          DellExportUse     = "Replace"
	DellIncludeDefault         DellExportInclude = "Default"
	DellIncludeReadOnly        DellExportInclude = "IncludeReadOnly"
	DellIncludePasswordHashes  DellExportInclude = "IncludePasswordHashValues"
	DellIncludeCustomTelemetry DellExportInclude = "IncludeCustomTelemetry"
	DellShareLocal             DellShareType     = "LOCAL"
	DellShareNFS               DellShareType     = "NFS"
	DellShareCIFS              DellShareType     = "CIFS"
	DellShareHTTP              DellShareType     = "HTTP"
	DellShareHTTPS             DellShareType     = "HTTPS"
)

// DellShareParameters selects exported components and an optional remote share.
type DellShareParameters struct {
	Target                   string        `json:"Target"`
	ShareType                DellShareType `json:"ShareType,omitempty"`
	IPAddress                string        `json:"IPAddress,omitempty"`
	ShareName                string        `json:"ShareName,omitempty"`
	FileName                 string        `json:"FileName,omitempty"`
	UserName                 string        `json:"UserName,omitempty"`
	Password                 string        `json:"Password,omitempty"`
	Workgroup                string        `json:"Workgroup,omitempty"`
	IgnoreCertificateWarning *string       `json:"IgnoreCertificateWarning,omitempty"`
}

// DellExportSystemConfigurationRequest starts a Server Configuration Profile export.
type DellExportSystemConfigurationRequest struct {
	ExportFormat    DellExportFormat    `json:"ExportFormat,omitempty"`
	ExportUse       DellExportUse       `json:"ExportUse,omitempty"`
	IncludeInExport DellExportInclude   `json:"IncludeInExport,omitempty"`
	ShareParameters DellShareParameters `json:"ShareParameters"`
}

func (m DellShareParameters) Format(state fmt.State, verb rune) { formatSensitiveModel(state, m) }
func (m DellExportSystemConfigurationRequest) Format(state fmt.State, verb rune) {
	formatSensitiveModel(state, m)
}
