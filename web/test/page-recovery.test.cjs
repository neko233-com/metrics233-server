const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), vm = require("node:vm"), ts = require("typescript");
const React = require("react"), { renderToStaticMarkup } = require("react-dom/server");

test("a failed lazy page retains a safe explicit recovery action without automatic reload", () => {
  let reloads = 0;
  const exports = {};
  const output = ts.transpileModule(fs.readFileSync("src/ui/PageRecoveryBoundary.tsx", "utf8"), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  vm.runInNewContext(output, { exports, window: { location: { reload: () => reloads++ } }, require: name => {
    if (name === "../i18n") return { getLocale: () => "zh" };
    if (name === "./index") return { Alert: props => React.createElement("section", { role: "alert" }, props.children), Button: props => React.createElement("button", props, props.children) };
    return require(name);
  } });
  const Boundary = exports.PageRecoveryBoundary;
  const boundary = new Boundary({ children: React.createElement("p", null, "dashboard") });
  assert.match(renderToStaticMarkup(boundary.render()), /dashboard/);
  boundary.state = Boundary.getDerivedStateFromError(new Error("Failed to fetch dynamically imported module: private-resource"));
  const fallback = boundary.render();
  const markup = renderToStaticMarkup(fallback);
  assert.match(markup, /role="alert"/); assert.match(markup, /刷新页面/); assert.doesNotMatch(markup, /private-resource/);
  assert.equal(reloads, 0);
  const button = React.Children.toArray(fallback.props.children).find(child => typeof child === "object" && child.props?.onClick);
  button.props.onClick(); assert.equal(reloads, 1);
});
