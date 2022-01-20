

slice "cheese" {
} // It's totally the reblochon kind

boil "potatoes" {
  during = "30min"
} // no need to peel

stack "tartiflette" {
  in   = "cast iron pan"

  add {
    what     = boiled_potatoes
    quantity = "500G"
  }

  add {
    what     = sliced_cheese
    quantity = "400G" // just enough :)
  }

  // I don't have any onions ¯\_(ツ)_/¯
}
