package response

type Cookie struct {
	Domain   string `json:"domain"`
	Expiry   int64  `json:"expiry"`
	HTTPOnly bool   `json:"httpOnly"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	SameSite string `json:"sameSite"`
	Secure   bool   `json:"secure"`
	Value    string `json:"value"`
}

type Solution struct {
	URL       string   `json:"url"`
	Status    int      `json:"status"`
	Cookies   []Cookie `json:"cookies"`
	UserAgent string   `json:"userAgent"`
	Headers   struct{} `json:"headers"`
}

type ChallengeResponse struct {
	Status         string   `json:"status"`
	Message        string   `json:"message"`
	Solution       Solution `json:"solution"`
	StartTimestamp int64    `json:"startTimestamp"`
	EndTimestamp   int64    `json:"endTimestamp"`
	Version        string   `json:"version"`
}
