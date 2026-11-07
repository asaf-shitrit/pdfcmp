import { comparePdfToSnapshot } from 'pdf-visual-diff'
const [a, b, dir, tol] = process.argv.slice(2)
try {
  await comparePdfToSnapshot(a, dir, 'snap', { tolerance: Number(tol) })
  const same = await comparePdfToSnapshot(b, dir, 'snap', { tolerance: Number(tol) })
  console.log(same ? 'same' : 'diff')
} catch (e) { console.error(e); console.log('error') }
