const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { test } = require('node:test');
const { readSource, root, theme, themeRequire, ts } = require('./legacy-react-harness.cjs');

// A compound member such as Form.Field or Collapsible.Panel that the installed
// UI library does not define is undefined at runtime. Bundlers cannot detect it
// (only named imports are checked), and React then throws "Element type is
// invalid" for the whole page. This guard resolves every named import and every
// compound member read from the theme's UI libraries against the installed
// packages, so a missing component fails before release instead of in a browser.

const libraries = theme === 'air'
  ? ['@douyinfe/semi-ui', '@douyinfe/semi-icons']
  : ['@mui/material', '@mui/material/styles', '@mui/lab', '@mui/icons-material', '@mui/x-date-pickers', '@mui/x-date-pickers/AdapterDayjs', '@tabler/icons-react'];

/** sourceFiles returns every application source path under the supplied repository directory. */
function sourceFiles(directory) {
  const files = [];
  for (const entry of fs.readdirSync(path.join(root, directory), { withFileTypes: true })) {
    const relative = `${directory}/${entry.name}`;
    if (entry.isDirectory()) files.push(...sourceFiles(relative));
    else if (/\.jsx?$/.test(entry.name) && !/\.test\.jsx?$/.test(entry.name)) files.push(relative);
  }
  return files;
}

/** libraryReferences returns each named import and compound member that the supplied source reads from a checked UI library. */
function libraryReferences(relative) {
  const source = ts.createSourceFile(relative, readSource(relative), ts.ScriptTarget.Latest, true, ts.ScriptKind.JSX);
  const bindings = new Map();
  const references = [];
  /** location returns the source line of the supplied node for failure messages. */
  const location = node => `${relative}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}`;
  for (const statement of source.statements) {
    if (!ts.isImportDeclaration(statement) || !libraries.includes(statement.moduleSpecifier.text)) continue;
    const named = statement.importClause?.namedBindings;
    if (!named || !ts.isNamedImports(named)) continue;
    for (const element of named.elements) {
      const exported = (element.propertyName || element.name).text;
      bindings.set(element.name.text, [statement.moduleSpecifier.text, exported]);
      references.push({ at: location(element), path: [statement.moduleSpecifier.text, exported] });
    }
  }
  /** visit records compound component reads such as Form.Slot, Select.Option or const { Header } = Layout. */
  function visit(node) {
    if (ts.isPropertyAccessExpression(node) && ts.isIdentifier(node.expression) && bindings.has(node.expression.text) && /^[A-Z]/.test(node.name.text)) {
      references.push({ at: location(node), path: [...bindings.get(node.expression.text), node.name.text] });
    }
    if (ts.isVariableDeclaration(node) && ts.isObjectBindingPattern(node.name) && node.initializer && ts.isIdentifier(node.initializer) && bindings.has(node.initializer.text)) {
      for (const element of node.name.elements) {
        const member = (element.propertyName || element.name).getText(source);
        if (/^[A-Z]/.test(member)) references.push({ at: location(element), path: [...bindings.get(node.initializer.text), member] });
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  return references;
}

test(`${theme}: every UI library component the application reads is defined by the installed package`, () => {
  const missing = [];
  let checked = 0;
  for (const relative of sourceFiles(`web/${theme}/src`)) {
    for (const reference of libraryReferences(relative)) {
      const [library, ...members] = reference.path;
      let value = themeRequire(library);
      for (const member of members) value = value?.[member];
      checked++;
      if (value === undefined) missing.push(`${reference.at}: ${reference.path.join('.')}`);
    }
  }
  assert.ok(checked >= 200, `Inspect the application rather than an empty selection (${checked} references)`);
  assert.deepEqual(missing, [], 'Use components that the installed UI library defines');
});
