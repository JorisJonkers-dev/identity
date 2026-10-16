// Stands in for a Client's host that serves its icon: /board.png is a 1x1 PNG, anything else 404.
import { createServer } from 'node:http'

const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64')

createServer((req, res) => {
  if (req.url === '/board.png') {
    res.writeHead(200, { 'content-type': 'image/png' }).end(png)
    return
  }
  res.writeHead(req.url === '/healthz' ? 200 : 404).end()
}).listen(18603, 'localhost')
