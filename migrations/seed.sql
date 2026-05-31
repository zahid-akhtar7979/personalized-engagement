-- Seed categories
INSERT INTO categories (id, name, parent_id) VALUES
(1, 'Electronics', NULL),
(2, 'Computers', 1),
(3, 'Phones', 1),
(4, 'Fashion', NULL),
(5, 'Home', NULL),
(6, 'Sports', NULL)
ON CONFLICT (id) DO NOTHING;

-- Seed demo users
INSERT INTO users (id, username, email) VALUES
(1, 'demo_user_1', 'user1@demo.com'),
(2, 'demo_user_2', 'user2@demo.com'),
(3, 'demo_user_3', 'user3@demo.com'),
(4, 'demo_user_4', 'user4@demo.com'),
(5, 'demo_user_5', 'user5@demo.com')
ON CONFLICT (id) DO NOTHING;

-- Seed content catalog (product-style mock images)
INSERT INTO content_catalog (item_id, title, category_id, category, price, image_url) VALUES
(1001, 'Pro Laptop 15"', 2, 'Computers', 1299.99, 'https://cdn.dummyjson.com/product-images/laptops/apple-macbook-pro-14-inch-space-grey/thumbnail.webp'),
(1002, 'Wireless Mouse', 2, 'Computers', 49.99, 'https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods/thumbnail.webp'),
(1003, 'USB-C Hub', 2, 'Computers', 79.99, 'https://cdn.dummyjson.com/product-images/laptops/asus-zenbook-pro-dual-screen-laptop/thumbnail.webp'),
(1004, 'Smartphone X', 3, 'Phones', 899.99, 'https://cdn.dummyjson.com/product-images/smartphones/iphone-13-pro/thumbnail.webp'),
(1005, 'Phone Case', 3, 'Phones', 24.99, 'https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods-max-silver/thumbnail.webp'),
(1006, 'Running Shoes', 6, 'Sports', 129.99, 'https://cdn.dummyjson.com/product-images/mens-shoes/nike-air-jordan-1-red-and-black/thumbnail.webp'),
(1007, 'Yoga Mat', 6, 'Sports', 39.99, 'https://cdn.dummyjson.com/product-images/sports-accessories/baseball-glove/thumbnail.webp'),
(1008, 'Desk Lamp', 5, 'Home', 59.99, 'https://cdn.dummyjson.com/product-images/home-decoration/house-showpiece-plant/thumbnail.webp'),
(1009, 'Coffee Maker', 5, 'Home', 89.99, 'https://cdn.dummyjson.com/product-images/kitchen-accessories/black-aluminium-cup/thumbnail.webp'),
(1010, 'Winter Jacket', 4, 'Fashion', 199.99, 'https://cdn.dummyjson.com/product-images/mens-shirts/man-plaid-shirt/thumbnail.webp'),
(1011, 'Bluetooth Headphones', 1, 'Electronics', 149.99, 'https://cdn.dummyjson.com/product-images/mobile-accessories/apple-airpods-max-silver/thumbnail.webp'),
(1012, '4K Monitor', 2, 'Computers', 449.99, 'https://cdn.dummyjson.com/product-images/laptops/huawei-matebook-x-pro/thumbnail.webp'),
(1013, 'Fitness Tracker', 6, 'Sports', 79.99, 'https://cdn.dummyjson.com/product-images/mens-watches/brown-leather-belt-watch/thumbnail.webp'),
(1014, 'Smart Watch', 3, 'Phones', 299.99, 'https://cdn.dummyjson.com/product-images/mens-watches/longines-master-collection/thumbnail.webp'),
(1015, 'Tablet Pro', 2, 'Computers', 599.99, 'https://cdn.dummyjson.com/product-images/tablets/ipad-mini-2021-starlight/thumbnail.webp')
ON CONFLICT (item_id) DO NOTHING;
