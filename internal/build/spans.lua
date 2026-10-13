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

-- Every bracketed citation, so the build can stop one that has nothing to be
-- cited from: pandoc would print "[@key]" as written. A bare @key is how
-- people write a handle, and left alone.
function Cite(el)
  for _, c in ipairs(el.citations) do
    if c.mode ~= "AuthorInText" then
      io.stderr:write("MDBRAND-CITE " .. c.id .. "\n")
    end
  end
  return nil
end

function Div(el)
  check(el)
  if el.classes:includes("accent") then
    report(el, "a ::: {.accent} block")
  end
  return nil
end

-- A link to an anchor no element carries prints as text that goes nowhere:
-- [the setup](#setup) beside a heading "Set-up", whose id is set-up. Every
-- identifier pandoc assigned is gathered, and each #target without one is
-- reported with the identifiers that exist, so the build can name the nearest.
function Pandoc(doc)
  local ids, targets = {}, {}
  local function note(el)
    if el.identifier and el.identifier ~= "" then
      ids[el.identifier] = true
    end
  end
  doc:walk({
    Header = note, Div = note, Span = note, Code = note, CodeBlock = note,
    Image = note, Table = note, Figure = note,
    Link = function(el)
      note(el)
      if el.target:sub(1, 1) == "#" and #el.target > 1 then
        table.insert(targets, el.target:sub(2))
      end
    end,
  })
  local missing = false
  for _, t in ipairs(targets) do
    if not ids[t] then
      io.stderr:write("MDBRAND-ANCHOR " .. t .. "\n")
      missing = true
    end
  end
  if missing then
    local known = {}
    for id in pairs(ids) do
      table.insert(known, id)
    end
    table.sort(known)
    io.stderr:write("MDBRAND-IDS " .. table.concat(known, " ") .. "\n")
  end
  return nil
end

-- Raw HTML reaches neither output: the LaTeX and docx writers drop it, so an
-- <img> or a <table> vanished whole, a <br> joined two lines and <b> printed
-- its words plain, exit 0. Each opening tag is reported for the build to stop
-- naming its Markdown form; a comment is left alone, since dropping it is the
-- point, and so is a closing tag, which only repeats its opening one.
local function rawHTML(el)
  if el.format ~= "html" then
    return nil
  end
  local tag = el.text:match("^<%s*([%a][%w]*)")
  if tag then
    io.stderr:write("MDBRAND-HTML " .. tag:lower() .. "\t" .. el.text:gsub("%s+", " ") .. "\n")
  end
  return nil
end

RawInline = rawHTML
RawBlock = rawHTML
