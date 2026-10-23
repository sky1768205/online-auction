// Декоративные изображения сайта (обложки категорий, баннеры).
// Источник — Wikimedia Commons, свободные лицензии; авторство — в README.
const thumb = 'https://thumb.wikimedia.org/wikipedia/commons/thumb/'
const upload = 'https://upload.wikimedia.org/wikipedia/commons/'

export const images = {
  // И. И. Шишкин, «Зима» (1890) — фон первого слайда на главной
  hero: upload + 'd/d1/Ivan_Shishkin_-_Winter.JPG',
  // И. И. Шишкин, «На севере диком…» (1891)
  auth: thumb + '5/55/Shishkin_na_severe_dikom1.jpg/1280px-Shishkin_na_severe_dikom1.jpg',
  // Фарфор ЛФЗ «Кобальтовая сетка»
  seller: upload + '3/33/LFZ_chinaware_3.jpg',
  categories: {
    electronics: upload + 'b/b2/Majak_201.jpg',
    art: upload + 'c/cf/Forest_Landscape_with_Herons_%28Shishkin%29.jpg',
    collectibles: upload + '4/48/Lenin_badges_collection.jpg',
    watches: upload + 'b/bd/Poljot_Buran_chronograph_watch_from_about_1990_%28detail%29.jpg',
    books: thumb + '2/2f/Vintage_lamp_and_books_%28Unsplash%29.jpg/1280px-Vintage_lamp_and_books_%28Unsplash%29.jpg',
    home: thumb + '5/57/Lomonosov-porcelain_hg_01.jpg/1280px-Lomonosov-porcelain_hg_01.jpg',
  } as Record<string, string>,
}

export function categoryImage(slug: string): string {
  return images.categories[slug] ?? images.seller
}
