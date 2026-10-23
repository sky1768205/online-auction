import { calibrate } from '../realtime/clock'
import type {
  AdminStats,
  AdminUser,
  Bid,
  BidPlaced,
  Category,
  CreateLotRequest,
  DemoAccount,
  FeedBid,
  Lot,
  LotCancelled,
  Page,
  Participation,
  User,
} from './types'

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details?: Record<string, unknown>,
  ) {
    super(message)
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin' }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    init.body = JSON.stringify(body)
    init.headers = { 'Content-Type': 'application/json' }
  }

  const sentAt = Date.now()
  let res: Response
  try {
    res = await fetch('/api' + path, init)
  } catch {
    throw new ApiError(0, 'network', 'Нет соединения с сервером')
  }
  if (res.status === 204) return undefined as T

  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new ApiError(res.status, data.error ?? 'http_error', data.message ?? `Ошибка ${res.status}`, data.details)
  }
  if (typeof data?.serverTime === 'string') calibrate(data.serverTime, sentAt, Date.now())
  return data as T
}

function query(params: Record<string, string | number | undefined>) {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') q.set(k, String(v))
  const s = q.toString()
  return s ? '?' + s : ''
}

export const api = {
  me: () => request<User>('GET', '/auth/me'),
  login: (email: string, password: string) => request<User>('POST', '/auth/login', { email, password }),
  register: (email: string, password: string, displayName: string) =>
    request<User>('POST', '/auth/register', { email, password, displayName }),
  logout: () => request<void>('POST', '/auth/logout'),
  demoAccounts: () => request<DemoAccount[]>('GET', '/auth/demo'),
  demoLogin: (role: string) => request<User>('POST', '/auth/demo', { role }),

  categories: () => request<Category[]>('GET', '/categories'),
  lots: (params: { category?: string; status?: string; q?: string; sort?: string; page?: number; limit?: number }) =>
    request<Page<Lot>>('GET', '/lots' + query(params)),
  lot: (id: number) => request<{ lot: Lot; serverTime: string }>('GET', `/lots/${id}`),
  bids: (id: number, limit = 50) => request<Bid[]>('GET', `/lots/${id}/bids?limit=${limit}`),
  placeBid: (id: number, amount: number) => request<BidPlaced>('POST', `/lots/${id}/bids`, { amount }),
  myBids: () => request<{ items: Participation[] }>('GET', '/me/bids'),
  myLots: () => request<{ items: Lot[] }>('GET', '/me/lots'),
  createLot: (body: CreateLotRequest) => request<Lot>('POST', '/lots', body),
  upload: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return request<{ url: string }>('POST', '/uploads', form)
  },

  admin: {
    stats: () => request<AdminStats>('GET', '/admin/stats'),
    bids: () => request<FeedBid[]>('GET', '/admin/bids'),
    lots: (params: { status?: string; q?: string }) => request<{ items: Lot[]; total: number }>('GET', '/admin/lots' + query(params)),
    cancelLot: (id: number, reason: string) => request<LotCancelled>('POST', `/admin/lots/${id}/cancel`, { reason }),
    users: (q = '') => request<AdminUser[]>('GET', '/admin/users' + query({ q })),
    blockUser: (id: number, cancelLots: boolean) => request<{ cancelledLots: number }>('POST', `/admin/users/${id}/block`, { cancelLots }),
    unblockUser: (id: number) => request<void>('POST', `/admin/users/${id}/unblock`),
  },
}
