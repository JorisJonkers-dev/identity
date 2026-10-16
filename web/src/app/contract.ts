import { Ajv2020, type ValidateFunction } from 'ajv/dist/2020.js'
import addFormats from 'ajv-formats'
import { parse } from 'yaml'
import source from '../../../openapi/v1/openapi.yaml?raw'

/**
 * Checks a test's stubbed answer against the OpenAPI contract, so a page test cannot pass on an
 * answer the server would never send. Test tooling only: the contract is bundled into the tests.
 */

type Media = { schema?: object }
type Answer = { $ref?: string; content?: Record<string, Media> }
type Operation = { responses: Record<string, Answer> }
type Contract = {
  paths: Record<string, Record<string, Operation>>
  components: { responses: Record<string, Answer> }
}

const contract = parse(source) as Contract

const ajv = new Ajv2020({ strict: false, allErrors: true })
addFormats(ajv)
ajv.addSchema(contract, 'contract')
const compiled = new Map<string, ValidateFunction>()

/** The path template a concrete path matches; no two of the contract's templates overlap. */
function template(path: string): string | undefined {
  return Object.keys(contract.paths).find((t) => new RegExp(`^${t.replace(/\{[^}]+\}/g, '[^/]+')}$`).test(path))
}

/**
 * The answer an operation gives for a status: exact, then its range (4XX), then default, which the
 * contract's lint makes every operation declare.
 */
function answerFor(operation: Operation, status: number): [Answer, string] {
  const key = [String(status), `${String(Math.floor(status / 100))}XX`].find((k) => k in operation.responses) ?? 'default'
  const answer = operation.responses[key] as Answer
  if (answer.$ref) {
    const name = answer.$ref.split('/').pop() as string
    return [contract.components.responses[name] as Answer, `/components/responses/${name}`]
  }
  return [answer, `/paths/${pointer(key)}`]
}

function pointer(segment: string): string {
  return segment.replaceAll('~', '~0').replaceAll('/', '~1')
}

/** Why the answer is not one the contract allows, or null when it is. */
export function checkStub(method: string, path: string, status: number, mediaType: string | null, body: unknown): string | null {
  const t = template(path)
  const operation = t === undefined ? undefined : contract.paths[t]?.[method.toLowerCase()]
  if (t === undefined || !operation) return `the contract has no ${method} ${path}`
  const [answer, base] = answerFor(operation, status)
  if (!answer.content) return body == null ? null : `${method} ${path} answers ${String(status)} with no body`
  const declared = Object.keys(answer.content)
  if (!mediaType || !declared.includes(mediaType)) {
    return `${method} ${path} answers ${String(status)} as ${declared.join(' or ')}, not ${mediaType ?? 'no body'}`
  }
  const at = base.startsWith('/paths/')
    ? `/paths/${pointer(t)}/${method.toLowerCase()}/responses${base.slice('/paths'.length)}/content/${pointer(mediaType)}/schema`
    : `${base}/content/${pointer(mediaType)}/schema`
  let validate = compiled.get(at)
  if (!validate) {
    validate = ajv.compile({ $ref: `contract#${at}` })
    compiled.set(at, validate)
  }
  if (validate(body)) return null
  return `${method} ${path} ${String(status)}: ${ajv.errorsText(validate.errors)}`
}
