package checkout

type CreateCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url,omitempty"`
	Upgraded    bool   `json:"upgraded,omitempty"`
	PlanSlug    string `json:"plan_slug,omitempty"`
}
