/// Legacy suggestion replies have no query ID. Serialize requests and invalidate
/// their edit generation instead of letting an older response overwrite a draft.
#[derive(Default)]
pub struct Suggestions {
    epoch: u64,
    in_flight: Option<(u64, String)>,
    pending: Option<String>,
}

impl Suggestions {
    pub fn query(&mut self, query: String) -> Option<String> {
        if self.in_flight.is_some() {
            self.pending = Some(query);
            None
        } else {
            self.in_flight = Some((self.epoch, query.clone()));
            Some(query)
        }
    }
    pub fn clear(&mut self) {
        self.epoch = self.epoch.wrapping_add(1);
        self.pending = None;
    }
    pub fn reply(&mut self) -> (bool, Option<String>) {
        let current = self
            .in_flight
            .take()
            .is_some_and(|(epoch, _)| epoch == self.epoch);
        let pending = self.pending.take();
        let accept = current && pending.is_none();
        (accept, pending.and_then(|q| self.query(q)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn coalesces_without_showing_old_queries() {
        let mut s = Suggestions::default();
        assert_eq!(s.query("a".into()), Some("a".into()));
        assert_eq!(s.query("ab".into()), None);
        assert_eq!(s.query("abc".into()), None);
        assert_eq!(s.reply(), (false, Some("abc".into())));
        assert_eq!(s.reply(), (true, None));
    }
    #[test]
    fn reopening_cannot_accept_an_old_reply() {
        let mut s = Suggestions::default();
        s.query("old".into());
        s.clear();
        s.query("new".into());
        assert_eq!(s.reply(), (false, Some("new".into())));
        assert_eq!(s.reply(), (true, None));
        s.clear();
        assert_eq!(s.reply(), (false, None));
    }
}
