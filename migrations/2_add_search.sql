-- Full-text search for posts
CREATE VIRTUAL TABLE IF NOT EXISTS posts_fts USING fts5(
    title, 
    content, 
    content=posts, 
    content_rowid=id
);

-- Populate existing posts
INSERT INTO posts_fts(rowid, title, content)
SELECT id, title, content FROM posts;

-- Triggers to keep FTS in sync
CREATE TRIGGER posts_ai AFTER INSERT ON posts BEGIN
    INSERT INTO posts_fts(rowid, title, content) 
    VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER posts_ad AFTER DELETE ON posts BEGIN
    DELETE FROM posts_fts WHERE rowid = old.id;
END;

CREATE TRIGGER posts_au AFTER UPDATE ON posts BEGIN
    UPDATE posts_fts 
    SET title = new.title, content = new.content 
    WHERE rowid = old.id;
END;

-- Full-text search for pages
CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
    title, 
    content, 
    content=pages, 
    content_rowid=id
);

-- Populate existing pages
INSERT INTO pages_fts(rowid, title, content)
SELECT id, title, content FROM pages;

-- Triggers to keep FTS in sync
CREATE TRIGGER pages_ai AFTER INSERT ON pages BEGIN
    INSERT INTO pages_fts(rowid, title, content) 
    VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER pages_ad AFTER DELETE ON pages BEGIN
    DELETE FROM pages_fts WHERE rowid = old.id;
END;

CREATE TRIGGER pages_au AFTER UPDATE ON pages BEGIN
    UPDATE pages_fts 
    SET title = new.title, content = new.content 
    WHERE rowid = old.id;
END;