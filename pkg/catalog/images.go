package catalog

import (
	"strings"
)

// Tech / electronics product photos only (laptops, phones, tablets, accessories).
var techProductImages = []string{
	"https://cdn.dummyjson.com/product-images/laptops/apple-macbook-pro-14-inch-space-grey/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/laptops/asus-zenbook-pro-dual-screen-laptop/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/laptops/huawei-matebook-x-pro/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/smartphones/iphone-13-pro/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/smartphones/iphone-6/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/smartphones/iphone-5s/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/smartphones/samsung-galaxy-s8/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/tablets/ipad-mini-2021-starlight/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/tablets/samsung-galaxy-tab-s8-plus-grey/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/tablets/samsung-galaxy-tab-white/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods-max-silver/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/mobile-accessories/amazon-echo-plus/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/mens-watches/brown-leather-belt-watch/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/mens-watches/longines-master-collection/thumbnail.webp",
	"https://fakestoreapi.com/img/71z3kp+YqPL._AC_UL640_QL65_ML3_t.png",
	"https://fakestoreapi.com/img/61IBBVJvSDL._AC_SY879_t.png",
	"https://fakestoreapi.com/img/71YXzeOuslL._AC_UY879_t.png",
	"https://fakestoreapi.com/img/71li-ujtlUL._AC_UX679_t.png",
}

var laptopImages = []string{
	"https://cdn.dummyjson.com/product-images/laptops/apple-macbook-pro-14-inch-space-grey/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/laptops/asus-zenbook-pro-dual-screen-laptop/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/laptops/huawei-matebook-x-pro/thumbnail.webp",
}

var phoneImages = []string{
	"https://cdn.dummyjson.com/product-images/smartphones/iphone-13-pro/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/smartphones/iphone-6/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/smartphones/samsung-galaxy-s8/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/smartphones/iphone-5s/thumbnail.webp",
}

var accessoryImages = []string{
	"https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods-max-silver/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/mobile-accessories/amazon-echo-plus/thumbnail.webp",
	"https://cdn.dummyjson.com/product-images/tablets/ipad-mini-2021-starlight/thumbnail.webp",
}

// ImagePicker assigns unique product images across one recommendation response.
type ImagePicker struct {
	used map[string]bool
	slot int
}

func NewImagePicker() *ImagePicker {
	return &ImagePicker{used: make(map[string]bool)}
}

func (p *ImagePicker) URL(itemID int64, category string) string {
	url := PickProductImage(itemID, category, p.used, p.slot)
	p.slot++
	return url
}

// ProductImageURL returns a tech product image (no dedup).
func ProductImageURL(itemID int64, category string) string {
	return PickProductImage(itemID, category, nil, int(itemID%100))
}

// PickProductImage selects a laptop/phone/accessory image; avoids repeats when used map is set.
func PickProductImage(itemID int64, category string, used map[string]bool, slot int) string {
	pool := poolForCategory(category)
	if len(pool) == 0 {
		pool = techProductImages
	}
	start := int((itemID*31 + int64(slot)*17) % int64(len(pool)))
	if start < 0 {
		start = -start
	}
	for i := 0; i < len(pool); i++ {
		idx := (start + i) % len(pool)
		url := pool[idx]
		if used == nil || !used[url] {
			if used != nil {
				used[url] = true
			}
			return url
		}
	}
	return pool[start]
}

func poolForCategory(category string) []string {
	c := strings.ToLower(strings.TrimSpace(category))

	// Retailrocket categories — always electronics-style mocks, never groceries.
	if strings.HasPrefix(c, "category-") || c == "general" || c == "" {
		return techProductImages
	}
	if strings.Contains(c, "laptop") || strings.Contains(c, "computer") {
		return laptopImages
	}
	if strings.Contains(c, "phone") || strings.Contains(c, "mobile") {
		return phoneImages
	}
	if strings.Contains(c, "tablet") {
		return accessoryImages
	}
	if strings.Contains(c, "electronic") || strings.Contains(c, "accessor") {
		return accessoryImages
	}
	return techProductImages
}
