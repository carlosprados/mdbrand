-- deck is set by mdbrand when the document is a deck (style slides).
local deck = false
-- citeproc is unset by mdbrand when the document has no bibliography.
local citeproc = true
-- alt is set by mdbrand when the PDF must meet a standard: every picture is
-- then written with its description as alt text, which pandoc's writer
-- leaves out and PDF/UA requires.
local alt = false

-- typst.lua, run by pandoc before spans.lua when the engine is typst.
-- pandoc writes every @key as a bibliography citation; Typst numbers and
-- names figures, tables, sections and equations itself, so a key that is the
-- document's own label becomes a reference.

-- The words a reference prints before its number, as pandoc-crossref's
-- prefixes: "la [@tbl:x] resume" reads "la tabla 1.1 resume". mdbrand writes
-- the document's language's in.
local words = { fig = "figura", tbl = "tabla", eq = "ecuación", sec = "sección" }

-- asText prints a citation as written when there is no bibliography, as
-- XeLaTeX's writer does; Typst's #cite stopped the build for want of one.
-- spans.lua never sees it then, so the bracketed ones are reported here for
-- mdbrand to stop on, as spans.lua reports them; a bare @handle is text.
local function asText(c)
  for _, cit in ipairs(c.citations) do
    if cit.mode ~= "AuthorInText" then
      io.stderr:write("MDBRAND-CITE " .. cit.id .. "\n")
    end
  end
  return c.content
end

function Cite(c)
  if #c.citations ~= 1 then
    if not citeproc then return asText(c) end
    return nil
  end
  local cit = c.citations[1]
  local kind = cit.id:match("^(%a+):")
  if not kind or not words[kind] then
    if not citeproc then return asText(c) end
    return nil
  end
  if cit.mode == "SuppressAuthor" then
    return pandoc.RawInline("typst", "#ref(<" .. cit.id .. ">, supplement: none)")
  end
  return pandoc.RawInline("typst", "#ref(<" .. cit.id .. ">, supplement: [" .. words[kind] .. "])")
end

-- styled reports the text of a strong, an emph or a heading, code and math aside, for
-- mdbrand to check against the fallback face's styles: Typst prints a
-- character the fallback lacks the style for in its regular, and says nothing.
local function styled(style)
  return function(el)
    local text = pandoc.utils.stringify(el:walk({
      Code = function() return pandoc.Str("") end,
      Math = function() return pandoc.Str("") end,
    }))
    io.stderr:write("MDBRAND-STYLED " .. style .. " " .. text:gsub("\n", " ") .. "\n")
  end
end
Strong = styled("bold")
Emph = styled("italic")
Header = styled("heading")

-- $$ … $$ {#eq:x}: pandoc reads the label as text after the formula.
function Para(p)
  local c = p.content
  for i = 1, #c - 2 do
    if c[i].t == "Math" and c[i].mathtype == "DisplayMath" and c[i + 1].t == "Space" and c[i + 2].t == "Str" then
      local id = c[i + 2].text:match("^{#(eq:[^}]+)}$")
      if id then
        c[i + 1] = pandoc.RawInline("typst", " <" .. id .. ">")
        table.remove(c, i + 2)
        return p
      end
    end
  end
end

-- Typst resolves a picture against the .typ, in the work directory; pandoc
-- against its resource path. The first file that exists is written whole.
local function exists(f)
  local h = io.open(f, "r")
  if h then h:close() return true end
  return false
end
function Image(img)
  if img.src:match("^/") or img.src:match("^%a+://") then return nil end
  for _, dir in ipairs(PANDOC_STATE.resource_path) do
    local f = dir .. "/" .. img.src
    if exists(f) then
      img.src = f
      return img
    end
  end
end


-- A page break meant for the PDF is Typst's own; any other raw LaTeX is left
-- for spans.lua to report, and the build stops on it.
local breaks = { ["\\newpage"] = true, ["\\clearpage"] = true, ["\\pagebreak"] = true, ["\\cleardoublepage"] = true }
function RawBlock(r)
  if (r.format == "latex" or r.format == "tex") and breaks[r.text:match("^%s*(.-)%s*$")] then
    return pandoc.RawBlock("typst", "#pagebreak()")
  end
end

-- A table goes through the template's mdbrand-table with its row count: a
-- short one that fits is kept whole, since Typst has no control over the
-- rows a break leaves on either side.
function Table(t)
  local rows = 0
  for _, b in ipairs(t.bodies) do rows = rows + #b.body end
  local captioned = #t.caption.long > 0 and "true" or "false"
  return { pandoc.RawBlock("typst", "#mdbrand-table(" .. rows .. ", " .. captioned .. ")["), t, pandoc.RawBlock("typst", "]") }
end

-- A heading's first paragraph, when short, sticks to the next block too: a
-- heading and a one-line paragraph sat at the foot, the rest overleaf.
function Blocks(bs)
  if deck then return nil end
  local out = pandoc.Blocks({})
  for i, b in ipairs(bs) do
    if i > 1 and bs[i - 1].t == "Header" and b.t == "Para" and bs[i + 1] and #pandoc.utils.stringify(b) < 300 then
      out:insert(pandoc.RawBlock("typst", "#block(sticky: true)["))
      out:insert(b)
      out:insert(pandoc.RawBlock("typst", "]"))
    else
      out:insert(b)
    end
  end
  return out
end

-- ------------------------------------------------------------------ deck ---
-- A deck is cut into slides here, where beamer used to: each # is a section
-- slide, each ## a slide drawn by the template's mdbrand-slide, which
-- measures it and stops the build by its title when it does not fit. A
-- pause (. . .) or an incremental list makes one page per step, the slide's
-- number on each, what is still to come hidden — kept in the layout and out
-- of the text layer. Speaker notes leave the slide for notes-body.typ.

local function raw(s) return pandoc.RawBlock("typst", s) end
local function quote(s)
  s = s:gsub("\\", "\\\\"):gsub('"', '\\"')
  return '"' .. s .. '"'
end
local function isPause(b) return b.t == "Para" and pandoc.utils.stringify(b) == ". . ." end

-- units are a slide's blocks, each with the step it appears at.
-- As beamer counts them: a pause starts the next step, and an incremental
-- list's first item shows at the current one, each next item one step on.
local function units(blocks)
  local out, step, last = {}, 0, 0
  for _, b in ipairs(blocks) do
    if isPause(b) then
      step = last + 1
      last = step
    elseif b.t == "Div" and b.classes:includes("incremental") then
      for _, inner in ipairs(b.content) do
        if inner.t == "BulletList" or inner.t == "OrderedList" then
          for i, item in ipairs(inner.content) do
            local one = inner:clone()
            one.content = { item }
            table.insert(out, { block = one, step = step + i - 1 })
            last = math.max(last, step + i - 1)
          end
        else
          table.insert(out, { block = inner, step = step })
        end
      end
    else
      table.insert(out, { block = b, step = step })
    end
  end
  return out, last
end

-- columns turns a ::: columns div into a grid, a ::: column into a cell.
local function columns(blocks)
  local out = pandoc.Blocks({})
  for _, b in ipairs(blocks) do
    if b.t == "Div" and b.classes:includes("columns") then
      local widths, cells = {}, {}
      for _, c in ipairs(b.content) do
        if c.t == "Div" and c.classes:includes("column") then
          local w = c.attributes["width"]
          table.insert(widths, (w and w ~= "") and w or "1fr")
          table.insert(cells, c.content)
        end
      end
      out:insert(raw("#grid(columns: (" .. table.concat(widths, ", ") .. ",), column-gutter: 5mm,"))
      for _, cell in ipairs(cells) do
        out:insert(raw("["))
        out:extend(columns(cell))
        out:insert(raw("],"))
      end
      out:insert(raw(")"))
    else
      out:insert(b)
    end
  end
  return out
end

local function images(blocks)
  local found = {}
  pandoc.Blocks(blocks):walk({ Image = function(i) table.insert(found, quote(i.src:match("[^/]+$"))) end })
  if #found == 0 then return "()" end
  return "(" .. table.concat(found, ", ") .. ",)"
end

-- slideLevel is pandoc's: the highest heading level followed straight by
-- content. Above it a heading is a section slide; at it, a slide; below it,
-- a heading on the slide.
local function slideLevel(blocks)
  local level = 6
  for i, b in ipairs(blocks) do
    local next = blocks[i + 1]
    if b.t == "Header" and next and next.t ~= "Header" then
      level = math.min(level, b.level)
    end
  end
  return level
end

-- A picture written with its alt text: a figure's is its caption, any
-- other's the description in ![…](…). One without is reported, by file.
local lengthUnits = { mm = true, cm = true, ["in"] = true, pt = true, em = true }
local function typstString(t) return '"' .. t:gsub('\\', '\\\\'):gsub('"', '\\"') .. '"' end
local function withAlt(text, inline)
  return function(img)
    local args = { typstString(img.src) }
    for _, k in ipairs({ "width", "height" }) do
      local v = img.attributes[k]
      if v then
        local n, u = v:match("^([%d.]+)(%a*%%?)$")
        if not n or not (lengthUnits[u] or u == "%") then return nil end
        table.insert(args, k .. ": " .. n .. u)
      end
    end
    local a = text or pandoc.utils.stringify(img.caption)
    a = a:gsub("%s+", " "):match("^%s*(.-)%s*$")
    if a == "" then
      io.stderr:write("MDBRAND-NOALT " .. img.src .. "\n")
    else
      table.insert(args, "alt: " .. typstString(a))
    end
    local call = "image(" .. table.concat(args, ", ") .. ")"
    return pandoc.RawInline("typst", inline and "#box(" .. call .. ")" or "#" .. call)
  end
end

function Pandoc(doc)
  if alt and not deck then
    doc = doc:walk({ Figure = function(f)
      local caption = pandoc.utils.stringify(f.caption.long or f.caption)
      f.content = f.content:walk({ Image = withAlt(caption ~= "" and caption or nil, false) })
      return f
    end })
    doc = doc:walk({ Image = withAlt(nil, true), Math = function(m)
      -- An equation is read out from its TeX, as MathJax reads it.
      local w = pandoc.write(pandoc.Pandoc({ pandoc.Plain({ m }) }), "typst"):gsub("%s+$", "")
      return pandoc.RawInline("typst", "#math.equation(block: " .. tostring(m.mathtype == "DisplayMath") ..
        ", alt: " .. typstString((m.text:gsub("%s+", " "):match("^%s*(.-)%s*$"))) .. ", " .. w .. ")")
    end })
    return doc
  end
  if not deck then return nil end
  local level = slideLevel(doc.blocks)
  -- Gather sections and slides, each slide's blocks and notes. Content before
  -- the first slide gets a slide without a title, never dropped.
  local items, cur = {}, nil
  for _, b in ipairs(doc.blocks) do
    if b.t == "Header" and b.level < level then
      cur = nil
      table.insert(items, { kind = "section", title = b.content })
    elseif b.t == "Header" and b.level == level then
      cur = { kind = "slide", title = b.content, blocks = {}, notes = {} }
      table.insert(items, cur)
    elseif b.t == "RawBlock" and b.format == "html" and b.text:match("^%s*<!%-%-") then
      -- a comment prints nothing, and makes no slide
    else
      if not cur then
        cur = { kind = "slide", title = {}, blocks = {}, notes = {} }
        table.insert(items, cur)
      end
      if b.t == "Div" and b.classes:includes("notes") then
        for _, n in ipairs(b.content) do table.insert(cur.notes, n) end
      else
        table.insert(cur.blocks, b)
      end
    end
  end
  local total = 1 + #items
  local out = pandoc.Blocks({})
  local notes = {}
  local page, n = 1, 1
  for _, it in ipairs(items) do
    n = n + 1
    if it.kind == "section" then
      page = page + 1
      out:insert(raw("#mdbrand-section(" .. n .. ", " .. total .. ")["))
      out:insert(pandoc.Plain(it.title))
      out:insert(raw("]"))
    else
      local us, steps = units(it.blocks)
      local name = quote(pandoc.utils.stringify(it.title))
      for k = 0, steps do
        page = page + 1
        local body = pandoc.Blocks({})
        for _, u in ipairs(us) do
          if u.step <= k then
            body:insert(u.block)
          else
            -- hide leaves a list's marker drawn: the markers go hidden too.
            body:insert(raw("#hide[#set list(marker: hide[■]); #set enum(numbering: n => hide[#n.])"))
            body:insert(u.block)
            body:insert(raw("]"))
          end
        end
        out:insert(raw("#mdbrand-slide(" .. n .. ", " .. total .. ", " .. name .. ", " .. images(it.blocks) .. ")["))
        out:insert(pandoc.Plain(it.title))
        out:insert(raw("]["))
        out:extend(columns(body))
        out:insert(raw("]"))
      end
      local text = pandoc.write(pandoc.Pandoc(columns(it.notes)), "typst")
      table.insert(notes, "#mdbrand-note(" .. n .. ", " .. total .. ", " .. page .. ", " .. name .. ")[\n" .. text .. "\n]\n")
    end
  end
  local f = io.open("notes-body.typ", "w")
  if f then
    f:write(table.concat(notes, "\n"))
    f:close()
  end
  doc.blocks = out
  return doc
end
