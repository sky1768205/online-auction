// Все денежные суммы — в копейках (целые числа), как на бэкенде.

export interface User {
  id: number
  email: string
  displayName: string
  role: 'user' | 'admin'
  createdAt: string
}

export interface DemoAccount {
  role: 'buyer' | 'seller' | 'admin'
  title: string
  description: string
  displayName: string
}

export interface Category {
  id: number
  name: string
  slug: string
}

export interface LotImage {
  id: number
  url: string
  position: number
}

export type LotStatus = 'active' | 'finished' | 'cancelled'

export interface Lot {
  id: number
  sellerId: number
  sellerName: string
  categoryId: number
  categorySlug: string
  categoryName: string
  title: string
  description: string
  startPrice: number
  minStep: number
  currentPrice: number | null
  leaderId: number | null
  leaderName: string | null
  bidCount: number
  minNextBid: number
  startsAt: string
  endsAt: string
  status: LotStatus
  winnerId: number | null
  winnerName: string | null
  finalPrice: number | null
  closedAt: string | null
  cancelReason: string | null
  coverUrl: string | null
  images?: LotImage[]
}

export interface Bid {
  id: number
  lotId: number
  userId: number
  userName: string
  amount: number
  createdAt: string
}

export interface Page<T> {
  items: T[]
  total: number
  page: number
  limit: number
  serverTime: string
}

// События WebSocket
export interface BidPlaced {
  lotId: number
  lotTitle: string
  bid: Bid
  currentPrice: number
  leaderId: number
  leaderName: string
  bidCount: number
  minNextBid: number
  endsAt: string
  extended: boolean
}

export interface LotClosed {
  lotId: number
  winnerId: number | null
  winnerName: string | null
  finalPrice: number | null
  closedAt: string
}

export interface LotClosedPersonal extends LotClosed {
  lotTitle: string
  sellerId: number
}

export interface Outbid {
  lotId: number
  lotTitle: string
  currentPrice: number
  minNextBid: number
  leaderName: string
  endsAt: string
}

export interface LotCancelled {
  lotId: number
  lotTitle: string
  sellerId: number
  reason: string
  closedAt: string
}

export interface Participation {
  lot: Lot
  myMaxBid: number
}

export interface CreateLotRequest {
  title: string
  description: string
  categoryId: number
  startPrice: number
  minStep: number
  durationMinutes: number
  images: string[]
}

/** Применяет событие bid:new к лоту (используется и в каталоге, и на странице лота). */
export function applyBid(lot: Lot, e: BidPlaced) {
  if (e.bidCount < lot.bidCount) return // устаревшее событие
  lot.currentPrice = e.currentPrice
  lot.leaderId = e.leaderId
  lot.leaderName = e.leaderName
  lot.bidCount = e.bidCount
  lot.minNextBid = e.minNextBid
  lot.endsAt = e.endsAt
}

export function applyCancelled(lot: Lot, e: LotCancelled) {
  lot.status = 'cancelled'
  lot.cancelReason = e.reason
  lot.closedAt = e.closedAt
}

export function applyClosed(lot: Lot, e: LotClosed) {
  lot.status = 'finished'
  lot.winnerId = e.winnerId
  lot.winnerName = e.winnerName
  lot.finalPrice = e.finalPrice
  lot.closedAt = e.closedAt
}

// ---------- Админ-панель ----------

export interface AdminStats {
  users: number
  blockedUsers: number
  activeLots: number
  finishedLots: number
  soldLots: number
  cancelledLots: number
  bids: number
  bids24h: number
  turnover: number
  online: number
}

export interface AdminUser {
  id: number
  email: string
  displayName: string
  role: 'user' | 'admin'
  blockedAt: string | null
  createdAt: string
  lots: number
  activeLots: number
  bids: number
  wins: number
}

export interface FeedBid {
  id: number
  lotId: number
  lotTitle: string
  userId: number
  userName: string
  amount: number
  createdAt: string
}
