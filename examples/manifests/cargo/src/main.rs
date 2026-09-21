fn main(){ let a: Vec<String> = std::env::args().skip(1).collect(); println!("cargo says hi {}", a.join(" ")); }
