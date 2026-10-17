-- spans.lua, run by pandoc for both outputs.
--
-- [text]{.accent} is set in the brand's accent colour: \textcolor in LaTeX,
-- the Accent character style in a .docx (Repair colours its runs directly as
-- well). Each one is reported as MDBRAND-ACCENT, so the build measures the
-- colour against the page only when a document prints words in it. Any other way of asking for a colour is reported, not converted:
-- pandoc drops color=, colour= and style= on every writer this tool uses, so
-- the words would simply print black. mdbrand reads the report from stderr
-- and stops the build, naming .accent.

-- pandoc's epub writers are epub3 and epub (2); mdbrand asks for epub3.
local epub = FORMAT:match("^epub") ~= nil

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

-- On an e-reader a code line wider than the screen wraps: each block's
-- widest line is reported, and the build warns past what a 6" column holds.
function CodeBlock(el)
  check(el)
  if epub then
    local widest = 0
    for line in (el.text .. "\n"):gmatch("([^\n]*)\n") do
      widest = math.max(widest, utf8.len(line) or #line)
    end
    local first = el.text:match("^[^\n]*") or ""
    io.stderr:write("MDBRAND-EPUB-CODE " .. widest .. "\t" .. first:sub(1, 50) .. "\n")
  end
end

-- A table's columns are each as wide as their longest word, or the word
-- breaks: reported by column, for the build to hold against the column.
function Table(el)
  if not epub then
    return nil
  end
  local longest = {}
  local function row(r)
    for i, cell in ipairs(r.cells) do
      for w in pandoc.utils.stringify(cell.contents):gmatch("%S+") do
        longest[i] = math.max(longest[i] or 0, utf8.len(w) or #w)
      end
    end
  end
  for _, r in ipairs(el.head.rows) do row(r) end
  for _, body in ipairs(el.bodies) do
    for _, r in ipairs(body.body) do row(r) end
  end
  -- pandoc's widths are shares of the A4 measure; on a screen the content
  -- sizes the columns. A cell may break a long word, or Play Books clips the
  -- table; a column of short words (a number, a code) is kept whole, or 3%
  -- of a phone's column broke "19" in two.
  for i, spec in ipairs(el.colspecs) do
    el.colspecs[i] = { spec[1], pandoc.ColWidthDefault }
  end
  local function mark(r)
    for i, cell in ipairs(r.cells) do
      if (longest[i] or 0) <= 5 then
        cell.attr.classes:insert("whole")
      end
    end
  end
  for _, r in ipairs(el.head.rows) do mark(r) end
  for _, body in ipairs(el.bodies) do
    for _, r in ipairs(body.body) do mark(r) end
  end
  local cols = {}
  for i = 1, #longest do
    cols[i] = tostring(longest[i] or 0)
  end
  local name = pandoc.utils.stringify(el.caption.long or {}):gsub("%s+", " ")
  if name == "" and el.head.rows[1] then
    local heads = {}
    for _, cell in ipairs(el.head.rows[1].cells) do
      table.insert(heads, pandoc.utils.stringify(cell.contents))
    end
    name = "| " .. table.concat(heads, " | ") .. " |"
  end
  io.stderr:write("MDBRAND-EPUB-TABLE " .. table.concat(cols, ",") .. "\t" .. name:sub(1, 60) .. "\n")
  return el
end
function Link(el) check(el) end

-- The bundle's named colours, [words]{.ok}, written in by mdbrand: each
-- one's hex and the private-use mark that opens it in a .pptx.
local named = {}

-- colour is the colour a span asks for by class, the accent first, with the
-- way each output sets it; nil when it asks for none. Any other class is
-- reported, so the build can name the colour a typo meant.
local function colour(el)
  local found
  for _, c in ipairs(el.classes) do
    if c == "accent" then
      found = found or { report = "MDBRAND-ACCENT", style = "Accent", tex = "\\textcolor{brandPrimary}{", mark = "\u{E000}" }
    elseif named[c] then
      found = found or { report = "MDBRAND-NAMED " .. c, style = "Colour-" .. c,
        tex = "\\textcolor[HTML]{" .. named[c].hex .. "}{", mark = named[c].mark }
    else
      io.stderr:write("MDBRAND-CLASS " .. c .. "\t" .. pandoc.utils.stringify(el):gsub("%s+", " "):sub(1, 50) .. "\n")
    end
  end
  return found
end

function Span(el)
  check(el)
  local c = colour(el)
  if not c then
    return nil
  end
  io.stderr:write(c.report .. "\n")
  -- pandoc's pptx writer has no character styles: private-use marks around
  -- the words survive into the run, where Repair colours them.
  if FORMAT == "pptx" then
    local out = pandoc.List({ pandoc.Str(c.mark) })
    out:extend(el.content)
    out:insert(pandoc.Str("\u{E001}"))
    return out
  end
  if FORMAT == "docx" then
    el.attributes["custom-style"] = c.style
    return el
  end
  -- The stylesheet colours the class.
  if epub then
    return el
  end
  local out = pandoc.List({ pandoc.RawInline("latex", c.tex) })
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
  -- A reflowed page has no breaks of its own to keep a block from.
  if epub then
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
  -- pandoc's pptx writer keeps two columns and drops the rest in silence.
  if FORMAT == "pptx" and el.classes:includes("columns") then
    local n = 0
    for _, b in ipairs(el.content) do
      if b.t == "Div" and b.classes:includes("column") then
        n = n + 1
      end
    end
    if n > 2 then
      io.stderr:write("MDBRAND-PPTX-COLUMNS " .. n .. "\t" .. pandoc.utils.stringify(el):sub(1, 50) .. "\n")
    end
  end
  if el.classes:includes("keep") then
    return keep(el)
  end
  -- The front matter's abstract, which mdbrand moves after the cover: as
  -- LaTeX's own environment, so babel names it (Resumen, Abstract).
  if el.classes:includes("abstract") and FORMAT ~= "docx" and not epub then
    local out = pandoc.List({ pandoc.RawBlock("latex", "\\begin{abstract}") })
    out:extend(el.content)
    out:insert(pandoc.RawBlock("latex", "\\end{abstract}"))
    return out
  end
  for _, c in ipairs(el.classes) do
    if c == "accent" or named[c] then
      report(el, "a ::: {." .. c .. "} block")
    end
  end
  return nil
end

-- A link to an anchor no element carries prints as text that goes nowhere:
-- [the setup](#setup) beside a heading "Set-up", whose id is set-up. Every
-- identifier pandoc assigned is gathered, and each #target without one is
-- reported with the identifiers that exist, so the build can name the nearest.
-- A slide of a .pptx holds one figure: pandoc's writer moves a second to a
-- slide of its own, untitled, which the PDF deck never had.
local function slideFigures(doc)
  local title, n = nil, 0
  local function flush()
    if n > 1 then
      io.stderr:write("MDBRAND-PPTX-FIGURES " .. n .. "\t" .. (title or "") .. "\n")
    end
  end
  for _, b in ipairs(doc.blocks) do
    if b.t == "Header" and b.level <= 2 then
      flush()
      title, n = pandoc.utils.stringify(b), 0
    elseif b.t == "Figure" or (b.t == "Para" and #b.content == 1 and b.content[1].t == "Image") then
      n = n + 1
    end
  end
  flush()
end

function Pandoc(doc)
  if FORMAT == "pptx" then
    slideFigures(doc)
  end
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
  if (FORMAT == "docx" or epub) and (el.format == "latex" or el.format == "tex") then
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
