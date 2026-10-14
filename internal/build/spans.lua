-- spans.lua, run by pandoc for both outputs.
--
-- [text]{.accent} is set in the brand's accent colour: \textcolor in LaTeX,
-- the Accent character style in a .docx (Repair colours its runs directly as
-- well). Each one is reported as MDBRAND-ACCENT, so the build measures the
-- colour against the page only when a document prints words in it. Any other way of asking for a colour is reported, not converted:
-- pandoc drops color=, colour= and style= on every writer this tool uses, so
-- the words would simply print black. mdbrand reads the report from stderr
-- and stops the build, naming .accent.

-- Matched whatever the case: pandoc keeps Color= as written, and drops it.
local refused = { color = true, colour = true, style = true, bgcolor = true, background = true }

local function report(el, what)
  local text = el.text or pandoc.utils.stringify(el)
  text = text:gsub("%s+", " ")
  io.stderr:write("MDBRAND-SPAN " .. what .. "\t" .. text .. "\n")
end

local function check(el)
  for k, v in pairs(el.attributes) do
    if refused[k:lower()] then
      report(el, k .. '="' .. v .. '"')
    end
  end
end

-- Any element that takes attributes drops these alike: a heading, inline
-- code, a code block or a link with {color=red} printed in black, exit 0.
function Header(el) check(el) end
function Code(el) check(el) end
function CodeBlock(el) check(el) end
function Link(el) check(el) end

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

-- ::: {.keep} is a block that does not break across pages: one that does not
-- fit in what is left of the page starts the next, whole. In LaTeX it is set
-- in a box first, so its height is known before a line of it is placed; that
-- height is reserved, as needspace does, and the box is unpacked so its notes
-- still reach the foot of the page. A block taller than a page is reported to
-- the log by number, and each block to stderr by number and name, so the
-- build can stop naming it. The macros are written before the first block,
-- and only then: a document without one keeps its .tex byte for byte.
local keepMacros = [[
\newsavebox\mdbrandKeepBox
\newdimen\mdbrandKeepHt
\newcommand\mdbrandKeepStart{\par\setbox\mdbrandKeepBox\vbox\bgroup}
\newcommand\mdbrandKeepEnd[1]{\par\egroup
  \mdbrandKeepHt=\dimexpr\ht\mdbrandKeepBox+\dp\mdbrandKeepBox\relax
  \ifdim\mdbrandKeepHt>\textheight
    \typeout{MDBRAND-KEEP-TOOTALL #1 \the\mdbrandKeepHt\space\the\textheight}\fi
  \penalty-100
  \ifdim\mdbrandKeepHt>\dimexpr\pagegoal-\pagetotal\relax
    \ifdim\dimexpr\pagegoal-\pagetotal\relax>0pt \vfil\penalty-10000\fi\fi
  \unvbox\mdbrandKeepBox}]]
local keeps = 0

local function keepName(el)
  for _, b in ipairs(el.content) do
    if b.t == "Header" then
      return pandoc.utils.stringify(b)
    end
  end
  local text = pandoc.utils.stringify(el):gsub("%s+", " ")
  if #text > 60 then
    text = text:sub(1, 57) .. "..."
  end
  return text
end

local function keep(el)
  local name = keepName(el)
  if FORMAT == "beamer" then
    io.stderr:write("MDBRAND-KEEP-DECK\t" .. name .. "\n")
    return nil
  end
  local nested = false
  el:walk({ RawBlock = function(r)
    if r.text:find("mdbrandKeepStart", 1, true) or r.text:find("_mdbrandKeep", 1, true) then
      nested = true
    end
  end })
  if nested then
    io.stderr:write("MDBRAND-KEEP-NESTED\t" .. name .. "\n")
    return nil
  end
  keeps = keeps + 1
  io.stderr:write("MDBRAND-KEEP " .. keeps .. "\t" .. name .. "\n")
  local out = pandoc.List()
  if FORMAT == "docx" then
    -- Bookmarks, which Word allows between paragraphs, mark the block for
    -- Repair: it sets keep-with-next and keep-lines-together on every
    -- paragraph inside, which Word and Google Docs both honour.
    local id = 900000 + keeps
    out:insert(pandoc.RawBlock("openxml", '<w:bookmarkStart w:id="' .. id .. '" w:name="_mdbrandKeep' .. keeps .. '"/>'))
    out:extend(el.content)
    out:insert(pandoc.RawBlock("openxml", '<w:bookmarkEnd w:id="' .. id .. '"/>'))
    return out
  end
  if keeps == 1 then
    out:insert(pandoc.RawBlock("latex", keepMacros))
  end
  out:insert(pandoc.RawBlock("latex", "\\mdbrandKeepStart"))
  out:extend(el.content)
  out:insert(pandoc.RawBlock("latex", "\\mdbrandKeepEnd{" .. keeps .. "}"))
  return out
end

function Div(el)
  check(el)
  if el.classes:includes("keep") then
    return keep(el)
  end
  -- The front matter's abstract, which mdbrand moves after the cover: as
  -- LaTeX's own environment, so babel names it (Resumen, Abstract).
  if el.classes:includes("abstract") and FORMAT ~= "docx" then
    local out = pandoc.List({ pandoc.RawBlock("latex", "\\begin{abstract}") })
    out:extend(el.content)
    out:insert(pandoc.RawBlock("latex", "\\end{abstract}"))
    return out
  end
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
  -- A .docx has no LaTeX: \newpage, \vspace or a \textcolor of the author's
  -- went missing from it without a word. Reported by command for a warning.
  if FORMAT == "docx" and (el.format == "latex" or el.format == "tex") then
    local cmd = el.text:match("\\[%a@]+") or el.text:sub(1, 30)
    io.stderr:write("MDBRAND-RAWTEX " .. cmd .. "\n")
    return nil
  end
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
