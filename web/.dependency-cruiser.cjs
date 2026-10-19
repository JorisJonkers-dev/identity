// The web app's layers, as depguard holds the Go core's (docs/adr/0011): infrastructure is the
// bottom, shared components sit on it, features on both, and the app wires features together.
// `pnpm lint` runs this; a new rule here is a new rule for every pull request.
/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: 'no-circular',
      severity: 'error',
      comment: 'A cycle makes two modules one: neither can change, load or be tested without the other.',
      // The generated API client is the generator's to arrange.
      from: { pathNot: '^src/infrastructure/api/' },
      to: { circular: true },
    },
    {
      name: 'infrastructure-is-the-bottom',
      severity: 'error',
      comment: 'The API client, HTTP and navigation know no component, feature or route.',
      from: { path: '^src/infrastructure/' },
      to: { path: '^src/(shared|features|app)/' },
    },
    {
      name: 'shared-knows-no-feature',
      severity: 'error',
      comment: 'A shared component serves every feature, so it may lean on none.',
      from: { path: '^src/shared/', pathNot: '\\.spec\\.ts$' },
      to: { path: '^src/(features|app)/' },
    },
    {
      name: 'features-do-not-import-the-app',
      severity: 'error',
      comment: 'The app wires features into routes; a feature that imports it imports every other feature.',
      from: { path: '^src/features/', pathNot: '\\.spec\\.ts$' },
      to: { path: '^src/app/' },
    },
    {
      name: 'source-does-not-import-tests',
      severity: 'error',
      comment: 'Specs and the test harness, which checks stubs against the contract, ship in no build.',
      from: { path: '^src/', pathNot: '\\.spec\\.ts$|^src/app/(testing|fixtures|contract)\\.ts$' },
      to: { path: '\\.spec\\.ts$|^src/app/(testing|fixtures|contract)\\.ts$' },
    },
    {
      name: 'no-dev-dependencies-in-the-build',
      severity: 'error',
      comment: 'What the browser runs comes from dependencies, never from devDependencies.',
      from: { path: '^src/', pathNot: '\\.spec\\.ts$|^src/app/(testing|fixtures|contract)\\.ts$|^src/env\\.d\\.ts$' },
      to: { dependencyTypes: ['npm-dev'], dependencyTypesNot: ['type-only'] },
    },
    {
      name: 'not-to-unresolvable',
      severity: 'error',
      comment: 'An import that resolves to nothing fails only at run time.',
      from: {},
      to: { couldNotResolve: true },
    },
    {
      name: 'no-undeclared-packages',
      severity: 'error',
      comment: 'A package used but not in package.json works only while something else pulls it in.',
      from: {},
      to: { dependencyTypes: ['npm-no-pkg', 'npm-unknown'] },
    },
  ],
  options: {
    doNotFollow: { path: 'node_modules' },
    tsPreCompilationDeps: true,
    tsConfig: { fileName: 'tsconfig.json' },
    enhancedResolveOptions: { exportsFields: ['exports'], conditionNames: ['import', 'require', 'node', 'default', 'types'], extensions: ['.ts', '.vue', '.js', '.json'] },
  },
}
