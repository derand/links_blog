// Suggests tags in the link form. A datalist suggests whole values, so each
// tag is prefixed with the ones already typed, and those are left out.
(function () {
  var input = document.getElementById("form-tags"), list = document.getElementById("form-tag-list");
  if (!input || !list) return;
  var tags = Array.from(list.options, function (o) { return o.value; });
  input.addEventListener("input", function () {
    var head = input.value.match(/^(?:.*,)?\s*/)[0];
    var used = new Set(head.split(",").map(function (t) { return t.trim().toLowerCase(); }));
    list.replaceChildren.apply(list, tags.filter(function (t) { return !used.has(t.toLowerCase()); })
      .map(function (t) { return new Option("", head + t); }));
  });
})();
