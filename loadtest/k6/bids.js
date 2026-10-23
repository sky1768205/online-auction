// Нагрузочный тест на k6 (альтернатива backend/cmd/loadtest).
//
//   brew install k6
//   k6 run loadtest/k6/bids.js
//   k6 run -e WATCHERS=2000 -e BIDDERS=100 -e DURATION=60s loadtest/k6/bids.js
//
// Сценарии:
//   watchers — держат WebSocket, подписаны на один лот, измеряют задержку bid:new;
//   bidders  — параллельно делают ставки через REST.
// Задержка считается от bid.createdAt (время сервера) до получения события,
// поэтому k6 и сервер должны работать на одной машине (общие часы).

import http from 'k6/http'
import { check, sleep } from 'k6'
import { WebSocket } from 'k6/websockets'
import { Counter, Rate, Trend } from 'k6/metrics'

const BASE = __ENV.BASE || 'http://localhost:8080'
const WS = BASE.replace(/^http/, 'ws') + '/ws'
const WATCHERS = Number(__ENV.WATCHERS || 500)
const BIDDERS = Number(__ENV.BIDDERS || 50)
const DURATION = __ENV.DURATION || '30s'

const broadcastLatency = new Trend('ws_bid_broadcast_latency', true)
const bidsAccepted = new Counter('bids_accepted')
const bidsRejected = new Counter('bids_rejected')
const bidUnexpected = new Rate('bid_unexpected_error')
const wsEvents = new Counter('ws_bid_events_received')

export const options = {
  setupTimeout: '120s',
  scenarios: {
    watchers: { executor: 'constant-vus', exec: 'watcher', vus: WATCHERS, duration: DURATION },
    bidders: { executor: 'constant-vus', exec: 'bidder', vus: BIDDERS, duration: DURATION, startTime: '3s' },
  },
  thresholds: {
    ws_bid_broadcast_latency: ['p(95)<200'],
    bid_unexpected_error: ['rate<0.01'],
    'http_req_duration{scenario:bidders}': ['p(95)<300'],
  },
}

const json = { headers: { 'Content-Type': 'application/json' } }

export function setup() {
  const run = Date.now()
  const users = []
  for (let i = 0; i <= BIDDERS; i++) {
    const u = { email: `k6-${run}-${i}@example.test`, password: `k6-pass-${run}-${i}`, displayName: `k6 ${i}` }
    const res = http.post(`${BASE}/api/auth/register`, JSON.stringify(u), json)
    check(res, { registered: (r) => r.status === 201 })
    users.push(u)
  }
  // После регистраций в cookie остался последний пользователь — входим продавцом users[0].
  http.post(`${BASE}/api/auth/login`, JSON.stringify(users[0]), json)
  const cats = http.get(`${BASE}/api/categories`).json()
  const lot = http
    .post(
      `${BASE}/api/lots`,
      JSON.stringify({
        title: `k6 нагрузочный тест ${run}`,
        categoryId: cats[0].id,
        startPrice: 10000,
        minStep: 1000,
        durationMinutes: 30,
        images: [],
      }),
      json,
    )
    .json()
  return { lotId: lot.id, minStep: lot.minStep, users: users.slice(1) }
}

export function watcher(data) {
  const ws = new WebSocket(WS)
  ws.onopen = () => ws.send(JSON.stringify({ type: 'subscribe', room: `lot:${data.lotId}` }))
  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data)
    if (msg.type === 'bid:new') {
      wsEvents.add(1)
      broadcastLatency.add(Date.now() - Date.parse(msg.data.bid.createdAt))
    }
  }
  // Держим соединение до конца сценария.
  ws.onclose = () => {}
  setTimeout(() => ws.close(), 60 * 60 * 1000)
}

let loggedIn = false
let minNext = 0

export function bidder(data) {
  if (!loggedIn) {
    const u = data.users[(__VU - 1) % data.users.length]
    http.post(`${BASE}/api/auth/login`, JSON.stringify({ email: u.email, password: u.password }), json)
    loggedIn = true
  }
  if (!minNext) minNext = http.get(`${BASE}/api/lots/${data.lotId}`).json().lot.minNextBid

  const amount = minNext + data.minStep * Math.floor(Math.random() * 3)
  const res = http.post(`${BASE}/api/lots/${data.lotId}/bids`, JSON.stringify({ amount }), json)
  if (res.status === 201) {
    bidsAccepted.add(1)
    minNext = res.json().minNextBid
    bidUnexpected.add(false)
  } else {
    const body = res.json() || {}
    const expected = ['bid_too_low', 'already_leading'].includes(body.error)
    bidUnexpected.add(!expected)
    bidsRejected.add(1, { reason: body.error || String(res.status) })
    if (body.details && body.details.minNextBid) minNext = body.details.minNextBid
  }
  sleep(Math.random() * 0.05)
}
