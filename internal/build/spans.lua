-- spans.lua, run by pandoc for both outputs.
--
-- [text]{.accent} is set in the brand's accent colour: \textcolor in LaTeX,
-- the Accent character style in a .docx (Repair colours its runs directly as
-- well). Each one is reported as MDBRAND-ACCENT, so the build measures the
-- colour against the page only when a document prints words in it. Any other way of asking for a colour is reported, not converted:
-- pandoc drops color=, colour= and style= on every writer this tool uses, so
-- the words would simply print black. mdbrand reads the report from stderr
-- and stops the build, naming .accent.

local refused = { "color", "colour", "style" }

local function report(el, what)
  local text = pandoc.utils.stringify(el):gsub("%s+", " ")
  io.stderr:write("MDBRAND-SPAN " .. what .. "\t" .. text .. "\n")
end

local function check(el)
  for _, k in ipairs(refused) do
    local v = el.attributes[k]
    if v then
      report(el, k .. '="' .. v .. '"')
    end
  end
end

function Span(el)
  check(el)
  if not el.classes:includes("accent") then
    return nil
  end
  io.stderr:write("MDBRAND-ACCENT\n")
  if FORMAT == "docx" then
    el.attributes["custom-style"] = "Accent"
    return el
  end
  local out = pandoc.List({ pandoc.RawInline("latex", "\\textcolor{brandPrimary}{") })
  out:extend(el.content)
  out:insert(pandoc.RawInline("latex", "}"))
  return out
end

function Div(el)
  check(el)
  if el.classes:includes("accent") then
    report(el, "a ::: {.accent} block")
  end
  return nil
end
