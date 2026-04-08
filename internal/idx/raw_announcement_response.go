package idx

import "time"

// RawAnnouncementResponse matches the exact API response structure
type rawAnnouncementResponse struct {
	ResultCount  int `json:"ResultCount"`
	SearchParams struct {
		DateFrom   string `json:"DateFrom"`
		DateTo     string `json:"DateTo"`
		Query      string `json:"Query"`
		Language   string `json:"Language"`
		KodeEmiten string `json:"KodeEmiten"`
		EmitenType string `json:"EmitenType"`
		SortOrder  string `json:"SortOrder"`
		SortColumn string `json:"SortColumn"`
		IndexFrom  int    `json:"indexfrom"`
		PageSize   int    `json:"pagesize"`
	} `json:"SearchParams"`
	Replies []struct {
		Pengumuman struct {
			ID                  int       `json:"Id"`
			OldFinalId          int       `json:"OldFinalId"`
			NoPengumuman        string    `json:"NoPengumuman"`
			FinalId             string    `json:"FinalId"`
			Id2                 string    `json:"Id2"`
			JudulPengumuman     string    `json:"JudulPengumuman"`
			JenisPengumuman     string    `json:"JenisPengumuman"`
			Kode_Emiten         string    `json:"Kode_Emiten"`
			Form_Id             string    `json:"Form_Id"`
			PerihalPengumuman   string    `json:"PerihalPengumuman"`
			JMSXGroupID         string    `json:"JMSXGroupID"`
			Divisi              string    `json:"Divisi"`
			KodeDivisi          string    `json:"KodeDivisi"`
			JenisEmiten         string    `json:"JenisEmiten"`
			EfekEmiten_DIRE     bool      `json:"EfekEmiten_DIRE"`
			EfekEmiten_DINFRA   bool      `json:"EfekEmiten_DINFRA"`
			EfekEmiten_Saham    bool      `json:"EfekEmiten_Saham"`
			EfekEmiten_Obligasi bool      `json:"EfekEmiten_Obligasi"`
			EfekEmiten_EBA      bool      `json:"EfekEmiten_EBA"`
			EfekEmiten_ETF      bool      `json:"EfekEmiten_ETF"`
			EfekEmiten_SPEI     bool      `json:"EfekEmiten_SPEI"`
			TglPengumuman       time.Time `json:"TglPengumuman"`
			CreatedDate         time.Time `json:"CreatedDate"`
		} `json:"pengumuman"`
		Attachments []struct {
			ID               int    `json:"Id"`
			PDFFilename      string `json:"PDFFilename"`
			FullSavePath     string `json:"FullSavePath"`
			JMSXGroupID      string `json:"JMSXGroupID"`
			CorrelationID    string `json:"CorrelationID"`
			OriginalFilename string `json:"OriginalFilename"`
			IsAttachment     bool   `json:"IsAttachment"`
		} `json:"attachments"`
	} `json:"Replies"`
}
