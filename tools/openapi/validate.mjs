import SwaggerParser from '@apidevtools/swagger-parser';
import { fileURLToPath } from 'node:url';

const path = fileURLToPath(new URL('../../docs/openapi.json', import.meta.url));
const api = await SwaggerParser.validate(path, {
  resolve: { external: false },
});
let operations = 0;
for (const item of Object.values(api.paths)) {
  operations += Object.keys(item).filter((key) =>
    ['get', 'post', 'put', 'patch', 'delete', 'head', 'options', 'trace'].includes(key),
  ).length;
}
console.log(`Valid OpenAPI ${api.openapi}: ${operations} operations, ${api.tags.length} module tags.`);
