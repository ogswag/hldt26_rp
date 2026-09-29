// Order keys follow the fractional-indexing npm package, the same algorithm as api/internal/ops/order.go.

const digits = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz'
const zero = digits[0]
const maxKeyLength = 128
const smallestInteger = 'A' + zero.repeat(26)

class OrderKeyError extends Error {}

function integerLength(head: string): number {
  if (head >= 'a' && head <= 'z') {
    return head.charCodeAt(0) - 'a'.charCodeAt(0) + 2
  }
  if (head >= 'A' && head <= 'Z') {
    return 'Z'.charCodeAt(0) - head.charCodeAt(0) + 2
  }
  throw new OrderKeyError(head)
}

function integerPart(key: string): string {
  const n = integerLength(key[0] ?? '')
  if (n > key.length) {
    throw new OrderKeyError(key)
  }
  return key.slice(0, n)
}

function validate(key: string): void {
  if (key === smallestInteger) {
    throw new OrderKeyError(key)
  }
  const i = integerPart(key)
  if (key.slice(i.length).endsWith(zero)) {
    throw new OrderKeyError(key)
  }
}

export function validOrderKey(key: string): boolean {
  if (key === '' || key.length > maxKeyLength) {
    return false
  }
  for (const ch of key) {
    if (!digits.includes(ch)) {
      return false
    }
  }
  try {
    validate(key)
    return true
  } catch {
    return false
  }
}

function midpoint(a: string, b: string | null): string {
  if (b !== null && a >= b) {
    throw new OrderKeyError(`${a} >= ${b}`)
  }
  if (a.endsWith(zero) || (b !== null && b.endsWith(zero))) {
    throw new OrderKeyError('trailing zero')
  }
  if (b !== null) {
    let n = 0
    while ((a[n] ?? zero) === b[n]) {
      n++
    }
    if (n > 0) {
      return b.slice(0, n) + midpoint(a.slice(n), b.slice(n))
    }
  }
  const digitA = a ? digits.indexOf(a[0]) : 0
  const digitB = b !== null && b !== '' ? digits.indexOf(b[0]) : digits.length
  if (digitB - digitA > 1) {
    return digits[Math.round(0.5 * (digitA + digitB))]
  }
  if (b !== null && b.length > 1) {
    return b.slice(0, 1)
  }
  return digits[digitA] + midpoint(a.slice(1), null)
}

function incrementInteger(x: string): string | null {
  if (integerLength(x[0]) !== x.length) {
    throw new OrderKeyError(x)
  }
  const [head, ...digs] = x.split('')
  let carry = true
  for (let i = digs.length - 1; carry && i >= 0; i--) {
    const d = digits.indexOf(digs[i]) + 1
    if (d === digits.length) {
      digs[i] = zero
    } else {
      digs[i] = digits[d]
      carry = false
    }
  }
  if (!carry) {
    return head + digs.join('')
  }
  if (head === 'Z') {
    return 'a' + zero
  }
  if (head === 'z') {
    return null
  }
  const h = String.fromCharCode(head.charCodeAt(0) + 1)
  if (h > 'a') {
    digs.push(zero)
  } else {
    digs.pop()
  }
  return h + digs.join('')
}

function decrementInteger(x: string): string | null {
  if (integerLength(x[0]) !== x.length) {
    throw new OrderKeyError(x)
  }
  const [head, ...digs] = x.split('')
  let borrow = true
  for (let i = digs.length - 1; borrow && i >= 0; i--) {
    const d = digits.indexOf(digs[i]) - 1
    if (d === -1) {
      digs[i] = digits[digits.length - 1]
    } else {
      digs[i] = digits[d]
      borrow = false
    }
  }
  if (!borrow) {
    return head + digs.join('')
  }
  if (head === 'a') {
    return 'Z' + digits[digits.length - 1]
  }
  if (head === 'A') {
    return null
  }
  const h = String.fromCharCode(head.charCodeAt(0) - 1)
  if (h < 'Z') {
    digs.push(digits[digits.length - 1])
  } else {
    digs.pop()
  }
  return h + digs.join('')
}

// keyBetween returns a key strictly between a and b; null means the start or the end.
export function keyBetween(a: string | null, b: string | null): string {
  if (a !== null) {
    validate(a)
  }
  if (b !== null) {
    validate(b)
  }
  if (a !== null && b !== null && a >= b) {
    throw new OrderKeyError(`${a} >= ${b}`)
  }
  if (a === null) {
    if (b === null) {
      return 'a' + zero
    }
    const ib = integerPart(b)
    const fb = b.slice(ib.length)
    if (ib === smallestInteger) {
      return ib + midpoint('', fb)
    }
    if (ib < b) {
      return ib
    }
    const res = decrementInteger(ib)
    if (res === null) {
      throw new OrderKeyError('cannot decrement')
    }
    return res
  }
  const ia = integerPart(a)
  const fa = a.slice(ia.length)
  if (b === null) {
    const i = incrementInteger(ia)
    return i === null ? ia + midpoint(fa, null) : i
  }
  const ib = integerPart(b)
  const fb = b.slice(ib.length)
  if (ia === ib) {
    return ia + midpoint(fa, fb)
  }
  const i = incrementInteger(ia)
  if (i === null) {
    throw new OrderKeyError('cannot increment')
  }
  if (i < b) {
    return i
  }
  return ia + midpoint(fa, null)
}

// keysAfter returns n increasing keys after a.
export function keysAfter(a: string | null, n: number): string[] {
  const out: string[] = []
  let prev = a
  for (let i = 0; i < n; i++) {
    prev = keyBetween(prev, null)
    out.push(prev)
  }
  return out
}
