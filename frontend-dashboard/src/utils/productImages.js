// Laptops, phones, tablets, accessories only — no groceries.

const techProductImages = [
  'https://cdn.dummyjson.com/product-images/laptops/apple-macbook-pro-14-inch-space-grey/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/laptops/asus-zenbook-pro-dual-screen-laptop/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/laptops/huawei-matebook-x-pro/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/smartphones/iphone-13-pro/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/smartphones/iphone-6/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/smartphones/iphone-5s/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/smartphones/samsung-galaxy-s8/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/tablets/ipad-mini-2021-starlight/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/tablets/samsung-galaxy-tab-s8-plus-grey/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/tablets/samsung-galaxy-tab-white/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods-max-silver/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/mobile-accessories/amazon-echo-plus/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/mens-watches/brown-leather-belt-watch/thumbnail.webp',
  'https://cdn.dummyjson.com/product-images/mens-watches/longines-master-collection/thumbnail.webp',
  'https://fakestoreapi.com/img/71z3kp+YqPL._AC_UL640_QL65_ML3_t.png',
  'https://fakestoreapi.com/img/61IBBVJvSDL._AC_SY879_t.png',
  'https://fakestoreapi.com/img/71YXzeOuslL._AC_UY879_t.png',
  'https://fakestoreapi.com/img/71li-ujtlUL._AC_UX679_t.png',
]

function poolForCategory(category = '') {
  const c = String(category).toLowerCase().trim()
  if (c.startsWith('category-') || c === 'general' || !c) {
    return techProductImages
  }
  if (c.includes('laptop') || c.includes('computer')) {
    return techProductImages.filter((u) => u.includes('/laptops/'))
  }
  if (c.includes('phone') || c.includes('mobile')) {
    return techProductImages.filter((u) => u.includes('/smartphones/'))
  }
  return techProductImages
}

/**
 * @param {number} itemId
 * @param {string} category
 * @param {number} slot - position in row (helps spread images)
 * @param {Set<string>} [used] - optional dedup across a row
 */
export function productImageUrl(itemId, category = '', slot = 0, used = null) {
  const pool = poolForCategory(category)
  const id = Math.abs(Number(itemId) || 0)
  const start = Math.abs((id * 31 + slot * 17) % pool.length)

  for (let i = 0; i < pool.length; i++) {
    const url = pool[(start + i) % pool.length]
    if (!used || !used.has(url)) {
      if (used) used.add(url)
      return url
    }
  }
  return pool[start]
}
